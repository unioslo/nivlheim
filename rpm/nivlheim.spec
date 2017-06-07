%global date 20170606

# Semantic Versioning http://semver.org/
Name:     nivlheim
Version:  0.1.0
Release:  %{date}%{?dist}

Summary:  File collector

Group:    Applications/System
License:  GPLv3+

URL:      https://github.com/oyvindhagberg/nivlheim
Source0:  https://github.com/oyvindhagberg/nivlheim/archive/master.zip

BuildRequires: perl(IO::Socket::INET6)
BuildRequires: perl(IO::Socket::SSL)
BuildRequires: perl(IO::File)
BuildRequires: perl(Socket)
BuildRequires: perl(Net::DNS)
BuildRequires: perl(NetAddr::IP)
BuildRequires: perl(Archive::Tar)
BuildRequires: perl(HTTP::Request::Common)
BuildRequires: perl(Sys::Syslog)
BuildRequires: perl(File::Path)
BuildRequires: perl(File::Basename)

Requires: perl, openssl
Requires: perl(IO::Socket::INET6)
Requires: perl(IO::Socket::SSL)
Requires: perl(IO::File)
Requires: perl(Socket)
Requires: perl(Net::DNS)
Requires: perl(NetAddr::IP)
Requires: perl(Archive::Tar)
Requires: perl(HTTP::Request::Common)
Requires: perl(Sys::Syslog)
Requires: perl(File::Path)
Requires: perl(File::Basename)

BuildArch: noarch

%global _binary_filedigest_algorithm 1
%global _source_filedigest_algorithm 1

%description
This package is the base package for Nivlheim, the file collector for
UiO.

%package client
Summary:  Client component of the file collector for UiO
Group:    Applications/System
Requires: %{name} = %{version}-%{release}

%package server
Summary:  Server components of the file collector for UiO
Group:    Applications/System
Requires: %{name} = %{version}-%{release}
Requires: httpd

%description client
This package contains the client component of Nivlheim, the file
collector for UiO.

%description server
This package contains the server components of Nivlheim, the file
collector for UiO.

%prep
%autosetup -n nivlheim-master

%build

%install
rm -rf %{buildroot}
mkdir -p %{buildroot}%{_sbindir}
mkdir -p %{buildroot}%{_sysconfdir}/nivlheim
mkdir -p %{buildroot}%{_sysconfdir}/httpd/conf.d
mkdir -p %{buildroot}%{_localstatedir}/local/nivlheim
install -p -m 0744 client/nivlheim_client %{buildroot}%{_sbindir}/
install -p -m 0644 client/client.conf %{buildroot}%{_sysconfdir}/nivlheim
install -p -m 0644 server/httpd_ssl.conf %{buildroot}%{_sysconfdir}/httpd/conf.d/nivlheim.conf

%check
perl -c %{buildroot}%{_sbindir}/nivlheim_client

%clean
rm -rf %{buildroot}

%files
%defattr(-, root, root, -)
%license LICENSE.txt
%dir %{_localstatedir}/local/nivlheim
%dir %{_sysconfdir}/nivlheim

%files client
%defattr(-, root, root, -)
%{_sbindir}/nivlheim_client
%config(noreplace) %{_sysconfdir}/nivlheim/client.conf

%files server
%defattr(-, root, root, -)
%config %{_sysconfdir}/httpd/conf.d/nivlheim.conf

%changelog
* Tue Jun  6 2017 Øyvind Hagberg <oyvind.hagberg@usit.uio.no> - 0.1.0
- First package build
