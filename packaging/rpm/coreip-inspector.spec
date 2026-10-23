Name:           coreip-server-inspector
Version:        1.0.0
Release:        1%{?dist}
Summary:        Production Linux server hardware inventory and health monitoring application
License:        Proprietary
URL:            https://github.com/coreip/server-inspector
Source0:        coreip-server-inspector-%{version}.tar.gz

BuildRequires:  systemd-rpm-macros
Requires:       systemd
Recommends:     smartmontools

%description
CoreIP Server Inspector provides local and secure remote web-based inspection
of Ubuntu and CentOS/RHEL servers, physical storage diagnostics, live monitoring,
and downloadable system reports without requiring external databases or runtimes.

%prep
%setup -q

%build
# Pre-compiled Go binary is packaged directly

%install
rm -rf %{buildroot}
install -d %{buildroot}%{_bindir}
install -d %{buildroot}%{_sysconfdir}/coreip-inspector
install -d %{buildroot}%{_unitdir}
install -d %{buildroot}%{_localstatedir}/log/coreip-inspector

install -m 0755 bin/coreip-inspector %{buildroot}%{_bindir}/coreip-inspector
install -m 0640 configs/config.yaml %{buildroot}%{_sysconfdir}/coreip-inspector/config.yaml
install -m 0644 configs/coreip-inspector.service %{buildroot}%{_unitdir}/coreip-inspector.service

%pre
if ! getent group coreip >/dev/null 2>&1; then
    groupadd --system coreip
fi
if ! getent passwd coreip >/dev/null 2>&1; then
    useradd --system --gid coreip --home-dir /var/lib/coreip-inspector --no-create-home --shell /sbin/nologin coreip
fi

%post
%systemd_post coreip-inspector.service
chown root:coreip %{_sysconfdir}/coreip-inspector/config.yaml || true
chown -R coreip:coreip %{_localstatedir}/log/coreip-inspector || true

%preun
%systemd_preun coreip-inspector.service

%postun
%systemd_postun_with_restart coreip-inspector.service

%files
%{_bindir}/coreip-inspector
%{_unitdir}/coreip-inspector.service
%config(noreplace) %attr(0640, root, coreip) %{_sysconfdir}/coreip-inspector/config.yaml
%dir %attr(0750, coreip, coreip) %{_localstatedir}/log/coreip-inspector

%changelog
* Fri Oct 24 2026 CoreIP Systems Engineering <support@coreip.local> - 1.0.0-1
- Initial production release of CoreIP Server Inspector
