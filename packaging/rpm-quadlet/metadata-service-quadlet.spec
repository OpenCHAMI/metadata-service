# SPDX-FileCopyrightText: 2026 OpenCHAMI Contributors
# SPDX-License-Identifier: MIT
#
# See `make rpm-build` and docs/RPM_PACKAGING.md for the tag-to-version
# mapping and how the packaged quadlet's image tag is pinned to it.

Name:           metadata-service-quadlet
Version:        %{version}
Release:        %{rel}%{?dist}
Summary:        OpenCHAMI metadata-service Quadlet units

License:        MIT
URL:            https://github.com/OpenCHAMI/metadata-service
Source0:        %{name}-%{version}.tar.gz

BuildArch:      noarch

Requires(post,preun,postun):  systemd
Requires:                     podman >= 4.4.0
Requires:                     smd-quadlet >= 2.20.0 tokensmith-quadlet >= 0.4.0

%description
Podman Quadlet unit files (container + volume) for running metadata-service
as part of an OpenCHAMI deployment.

%prep
%setup -q

%install
mkdir -p %{buildroot}/usr/share/containers/systemd
mkdir -p %{buildroot}/etc/openchami/configs

grep -q '@IMAGE_TAG@' metadata-service.container
sed "s|@IMAGE_TAG@|v%{version}|" metadata-service.container \
    > %{buildroot}/usr/share/containers/systemd/metadata-service.container
chmod 644 %{buildroot}/usr/share/containers/systemd/metadata-service.container

install -m 644 metadata-service-data.volume \
    %{buildroot}/usr/share/containers/systemd/metadata-service-data.volume
install -m 644 metadata-service.yaml \
    %{buildroot}/etc/openchami/configs/metadata-service.yaml

%files
%license LICENSES/MIT.txt
%dir /etc/openchami
%dir /etc/openchami/configs
%config(noreplace) /etc/openchami/configs/metadata-service.yaml
/usr/share/containers/systemd/metadata-service.container
/usr/share/containers/systemd/metadata-service-data.volume

%post
# reload systemd so the new Quadlet-generated unit is seen
systemctl daemon-reload || :
if [ $1 -ge 2 ]; then
    systemctl try-restart metadata-service.service || :
fi

%preun
if [ $1 -eq 0 ]; then
    systemctl stop metadata-service.service >/dev/null 2>&1 || :
fi

%postun
# reload systemd so the removed unit is dropped
systemctl daemon-reload || :
