Name:           iptvtunerr
Version:        0.1.63
Release:        1%{?dist}
Summary:        IPTV to Plex, Emby, and Jellyfin bridge

License:        AGPL-3.0-or-later
URL:            https://github.com/snapetech/iptvtunerr
Source0:        iptv-tunerr-v0.1.63-linux-amd64.tar.gz
Source1:        iptvtunerr.service
Source2:        iptvtunerr.env
Source3:        iptvtunerr.conf
Source4:        iptvtunerr.tmpfiles

BuildArch:      x86_64
BuildRequires:  systemd-rpm-macros
Requires(pre):  shadow-utils
Requires:       systemd
Recommends:     ffmpeg
%global debug_package %{nil}
%define __strip /bin/true

%description
IPTV Tunerr turns IPTV sources into a stable HDHomeRun-style tuner and XMLTV
guide surface for Plex, Emby, and Jellyfin.

%prep
%setup -q -c

%install
install -Dm755 iptv-tunerr-v%{version}-linux-amd64/iptv-tunerr %{buildroot}%{_bindir}/iptv-tunerr
install -Dm644 %{SOURCE1} %{buildroot}%{_unitdir}/iptvtunerr.service
install -Dm644 %{SOURCE2} %{buildroot}%{_sysconfdir}/iptvtunerr/iptvtunerr.env
install -Dm644 %{SOURCE3} %{buildroot}%{_sysusersdir}/iptvtunerr.conf
install -Dm644 %{SOURCE4} %{buildroot}%{_tmpfilesdir}/iptvtunerr.conf
install -dm750 %{buildroot}%{_sharedstatedir}/iptvtunerr
install -dm750 %{buildroot}%{_localstatedir}/cache/iptvtunerr
install -dm750 %{buildroot}%{_localstatedir}/log/iptvtunerr

%pre
getent passwd iptvtunerr >/dev/null || useradd -r -s /sbin/nologin -d %{_sharedstatedir}/iptvtunerr -c "IPTV Tunerr service account" iptvtunerr

%post
%tmpfiles_create %{_tmpfilesdir}/iptvtunerr.conf
%systemd_post iptvtunerr.service

%preun
%systemd_preun iptvtunerr.service

%postun
%systemd_postun_with_restart iptvtunerr.service

%files
%{_bindir}/iptv-tunerr
%{_unitdir}/iptvtunerr.service
%{_sysusersdir}/iptvtunerr.conf
%{_tmpfilesdir}/iptvtunerr.conf
%config(noreplace) %{_sysconfdir}/iptvtunerr/iptvtunerr.env
%dir %attr(0750,iptvtunerr,iptvtunerr) %{_sharedstatedir}/iptvtunerr
%dir %attr(0750,iptvtunerr,iptvtunerr) %{_localstatedir}/cache/iptvtunerr
%dir %attr(0750,iptvtunerr,iptvtunerr) %{_localstatedir}/log/iptvtunerr

%changelog
* Wed May 13 2026 snapetech <iptvtunerr@proton.me> - 0.1.63-1
- Initial package
