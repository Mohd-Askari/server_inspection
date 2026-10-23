#!/usr/bin/env bash
set -euo pipefail

VERSION="1.0.0"
PACKAGE_NAME="coreip-server-inspector"
ARCH="x86_64"
DIST_DIR="dist"

echo "=== Building RPM package for ${PACKAGE_NAME} v${VERSION} ==="

mkdir -p "${DIST_DIR}"

if ! command -v rpmbuild &>/dev/null; then
    echo "Notice: 'rpmbuild' is not installed on this host (common on non-RHEL / macOS)."
    echo "The RPM spec file is prepared at packaging/rpm/coreip-inspector.spec."
    echo "To build on a CentOS/RHEL system:"
    echo "  rpmbuild -ba packaging/rpm/coreip-inspector.spec"
    exit 0
fi

RPMBUILD_DIR=$(mktemp -d /tmp/rpmbuild.XXXXXX)
trap 'rm -rf "${RPMBUILD_DIR}"' EXIT

mkdir -p "${RPMBUILD_DIR}"/{BUILD,RPMS,SOURCES,SPECS,SRPMS}

# Build Linux binary
GOOS=linux GOARCH=amd64 go build -o bin/coreip-inspector ./cmd/inspector

# Create tarball source
SRC_TAR="${RPMBUILD_DIR}/SOURCES/${PACKAGE_NAME}-${VERSION}.tar.gz"
tar --transform "s,^,${PACKAGE_NAME}-${VERSION}/," -czf "${SRC_TAR}" bin configs packaging

cp packaging/rpm/coreip-inspector.spec "${RPMBUILD_DIR}/SPECS/"

rpmbuild --define "_topdir ${RPMBUILD_DIR}" -bb "${RPMBUILD_DIR}/SPECS/coreip-inspector.spec"

cp "${RPMBUILD_DIR}/RPMS/${ARCH}"/*.rpm "${DIST_DIR}/"
echo "RPM built successfully in ${DIST_DIR}/"
