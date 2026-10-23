package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	version     = "1.0.0"
	packageName = "coreip-server-inspector"
	arch        = "amd64"
)

func main() {
	distDir := "dist"
	if err := os.MkdirAll(distDir, 0755); err != nil {
		log.Fatalf("Failed to create dist directory: %v", err)
	}

	debPath := filepath.Join(distDir, fmt.Sprintf("%s_%s_%s.deb", packageName, version, arch))

	// 1. Build Linux amd64 binary first
	fmt.Println("Building Linux amd64 binary...")
	cmd := exec.Command("go", "build", "-o", "bin/coreip-inspector-linux-amd64", "./cmd/inspector")
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64")
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Fatalf("Go Linux build failed: %v\nOutput: %s", err, string(out))
	}

	// 2. Build control.tar.gz
	fmt.Println("Packing control.tar.gz...")
	controlBytes, err := buildControlTarGz()
	if err != nil {
		log.Fatalf("Failed building control.tar.gz: %v", err)
	}

	// 3. Build data.tar.gz
	fmt.Println("Packing data.tar.gz...")
	dataBytes, err := buildDataTarGz()
	if err != nil {
		log.Fatalf("Failed building data.tar.gz: %v", err)
	}

	// 4. Assemble ar archive (.deb)
	fmt.Println("Assembling .deb archive...")
	debFile, err := os.OpenFile(debPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatalf("Failed creating deb file: %v", err)
	}
	defer debFile.Close()

	if err := writeArArchive(debFile, []arEntry{
		{name: "debian-binary", data: []byte("2.0\n")},
		{name: "control.tar.gz", data: controlBytes},
		{name: "data.tar.gz", data: dataBytes},
	}); err != nil {
		log.Fatalf("Failed writing deb ar archive: %v", err)
	}

	fmt.Printf("Successfully generated Debian package: %s\n", debPath)
}

func buildControlTarGz() ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	controlContent, err := os.ReadFile("packaging/debian/control")
	if err != nil {
		return nil, err
	}

	postinstContent, err := os.ReadFile("packaging/debian/postinst")
	if err != nil {
		return nil, err
	}

	prermContent, err := os.ReadFile("packaging/debian/prerm")
	if err != nil {
		return nil, err
	}

	files := []struct {
		name string
		mode int64
		data []byte
	}{
		{"./control", 0644, controlContent},
		{"./postinst", 0755, postinstContent},
		{"./prerm", 0755, prermContent},
	}

	now := time.Now()
	for _, f := range files {
		hdr := &tar.Header{
			Name:     f.name,
			Mode:     f.mode,
			Size:     int64(len(f.data)),
			ModTime:  now,
			Typeflag: tar.TypeReg,
			Uname:    "root",
			Gname:    "root",
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if _, err := tw.Write(f.data); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildDataTarGz() ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	binaryData, err := os.ReadFile("bin/coreip-inspector-linux-amd64")
	if err != nil {
		return nil, err
	}

	configData, err := os.ReadFile("configs/config.yaml")
	if err != nil {
		return nil, err
	}

	serviceData, err := os.ReadFile("configs/coreip-inspector.service")
	if err != nil {
		return nil, err
	}

	now := time.Now()

	// Add directories
	dirs := []string{
		"./usr",
		"./usr/bin",
		"./etc",
		"./etc/coreip-inspector",
		"./lib",
		"./lib/systemd",
		"./lib/systemd/system",
		"./var",
		"./var/log",
		"./var/log/coreip-inspector",
	}

	for _, d := range dirs {
		hdr := &tar.Header{
			Name:     d + "/",
			Mode:     0755,
			ModTime:  now,
			Typeflag: tar.TypeDir,
			Uname:    "root",
			Gname:    "root",
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
	}

	files := []struct {
		name string
		mode int64
		data []byte
	}{
		{"./usr/bin/coreip-inspector", 0755, binaryData},
		{"./etc/coreip-inspector/config.yaml", 0640, configData},
		{"./lib/systemd/system/coreip-inspector.service", 0644, serviceData},
	}

	for _, f := range files {
		hdr := &tar.Header{
			Name:     f.name,
			Mode:     f.mode,
			Size:     int64(len(f.data)),
			ModTime:  now,
			Typeflag: tar.TypeReg,
			Uname:    "root",
			Gname:    "root",
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if _, err := tw.Write(f.data); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type arEntry struct {
	name string
	data []byte
}

func writeArArchive(w io.Writer, entries []arEntry) error {
	// Standard ar magic signature: "!<arch>\n"
	if _, err := io.WriteString(w, "!<arch>\n"); err != nil {
		return err
	}

	now := time.Now().Unix()
	for _, e := range entries {
		// Header: 16 bytes filename, 12 bytes mtime, 6 bytes uid, 6 bytes gid, 8 bytes mode, 10 bytes size, 2 bytes magic (`\x60\n`)
		// Filename in ar ends with '/'
		namePadded := fmt.Sprintf("%-16s", e.name)
		hdr := fmt.Sprintf("%-16s%-12d%-6d%-6d%-8o%-10d`\n",
			namePadded, now, 0, 0, 0644, len(e.data))
		if _, err := io.WriteString(w, hdr); err != nil {
			return err
		}
		if _, err := w.Write(e.data); err != nil {
			return err
		}
		// Ar archive entries must be 2-byte aligned
		if len(e.data)%2 != 0 {
			if _, err := w.Write([]byte("\n")); err != nil {
				return err
			}
		}
	}
	return nil
}

