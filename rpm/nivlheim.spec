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

BuildRequires: perl(Archive::Tar)
BuildRequires: perl(Archive::Zip)
BuildRequires: perl(CGI)
BuildRequires: perl(Crypt::OpenSSL::X509)
BuildRequires: perl(DateTime)
BuildRequires: perl(DBD::Pg)
BuildRequires: perl(DBI)
BuildRequires: perl(Encode)
BuildRequires: perl(File::Basename)
BuildRequires: perl(File::Copy)
BuildRequires: perl(File::Find)
BuildRequires: perl(File::Path)
BuildRequires: perl(File::Temp)
BuildRequires: perl(Getopt::Long)
BuildRequires: perl(HTTP::Request::Common)
BuildRequires: perl(IO::File)
BuildRequires: perl(IO::Socket::INET6)
BuildRequires: perl(IO::Socket::SSL)
BuildRequires: perl(IPC::Open3)
BuildRequires: perl(Log::Log4perl)
BuildRequires: perl(Log::Log4perl::Level)
BuildRequires: perl(MIME::Base64)
BuildRequires: perl(Net::CIDR)
BuildRequires: perl(Net::DNS)
BuildRequires: perl(Net::IP)
BuildRequires: perl(NetAddr::IP)
BuildRequires: perl(Proc::PID::File)
BuildRequires: perl(Socket)
BuildRequires: perl(Sys::Syslog)
BuildRequires: perl(Time::Piece)
BuildRequires: golang

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
Requires: perl, openssl
Requires: perl(Archive::Tar)
Requires: perl(File::Basename)
Requires: perl(File::Path)
Requires: perl(Getopt::Long)
Requires: perl(HTTP::Request::Common)
Requires: perl(IO::File)
Requires: perl(IO::Socket::INET6)
Requires: perl(IO::Socket::SSL)
Requires: perl(Net::DNS)
Requires: perl(NetAddr::IP)
Requires: perl(Socket)
Requires: perl(Sys::Syslog)

%package server
Summary:  Server components of the file collector for UiO
Group:    Applications/System
Requires: %{name} = %{version}-%{release}
Requires: perl, openssl, httpd, mod_ssl, postgresql, postgresql-server
Requires: golang, unzip, file
Requires: perl(Archive::Tar)
Requires: perl(Archive::Zip)
Requires: perl(CGI)
Requires: perl(Crypt::OpenSSL::X509)
Requires: perl(DateTime)
Requires: perl(DBD::Pg)
Requires: perl(DBI)
Requires: perl(Encode)
Requires: perl(File::Basename)
Requires: perl(File::Copy)
Requires: perl(File::Find)
Requires: perl(File::Temp)
Requires: perl(Log::Log4perl)
Requires: perl(Log::Log4perl::Level)
Requires: perl(MIME::Base64)
Requires: perl(Net::CIDR)
Requires: perl(Net::IP)
Requires: perl(Proc::PID::File)
Requires: perl(Time::Piece)

%{?systemd_requires}
BuildRequires: systemd

%description client
This package contains the client component of Nivlheim, the file
collector for UiO.

%description server
This package contains the server components of Nivlheim, the file
collector for UiO.

%prep
%autosetup -n nivlheim-master

%build
go build server/nivlheim_jobs.go

%install
rm -rf %{buildroot}
mkdir -p %{buildroot}%{_sbindir}
mkdir -p %{buildroot}%{_sysconfdir}/nivlheim
mkdir -p %{buildroot}%{_sysconfdir}/httpd/conf.d
mkdir -p %{buildroot}%{_localstatedir}/nivlheim
mkdir -p %{buildroot}/var/www/nivlheim
mkdir -p %{buildroot}/var/www/cgi-bin/secure
mkdir -p %{buildroot}/var/log/nivlheim
mkdir -p %{buildroot}/etc/systemd/system
install -p -m 0755 client/nivlheim_client %{buildroot}%{_sbindir}/
install -p -m 0644 client/client.conf %{buildroot}%{_sysconfdir}/nivlheim/
install -p -m 0644 server/httpd_ssl.conf %{buildroot}%{_sysconfdir}/httpd/conf.d/nivlheim.conf
install -p -m 0644 server/openssl_ca.conf %{buildroot}%{_sysconfdir}/nivlheim/
install -p -m 0755 server/ping %{buildroot}/var/www/cgi-bin/
install -p -m 0755 server/ping2 %{buildroot}/var/www/cgi-bin/secure/ping
install -p -m 0755 server/reqcert %{buildroot}/var/www/cgi-bin/
install -p -m 0755 server/renewcert %{buildroot}/var/www/cgi-bin/secure/
install -p -m 0755 server/post %{buildroot}/var/www/cgi-bin/secure/
install -p -m 0644 server/log4perl.conf %{buildroot}/var/www/nivlheim/
install -p -m 0755 server/nivlheim_setup.sh %{buildroot}%{_localstatedir}/nivlheim/
install -p -m 0644 server/init.sql %{buildroot}%{_localstatedir}/nivlheim/
install -p -m 0755 server/processarchive %{buildroot}/var/www/cgi-bin/
install -p -m 0755 nivlheim_jobs %{buildroot}%{_sbindir}
install -p -m 0644 server/nivlheim.service %{buildroot}%{_sysconfdir}/systemd/system/%{name}.service

%check
perl -c %{buildroot}%{_sbindir}/nivlheim_client
perl -c %{buildroot}/var/www/cgi-bin/secure/renewcert
perl -c %{buildroot}/var/www/cgi-bin/secure/ping
perl -c %{buildroot}/var/www/cgi-bin/secure/post
perl -c %{buildroot}/var/www/cgi-bin/ping
perl -c %{buildroot}/var/www/cgi-bin/reqcert
perl -c %{buildroot}/var/www/cgi-bin/processarchive

%clean
rm -rf %{buildroot}

%files
%defattr(-, root, root, -)
%license LICENSE.txt
%doc README.md
%dir %{_localstatedir}/nivlheim
%dir %{_sysconfdir}/nivlheim

%files client
%defattr(-, root, root, -)
%{_sbindir}/nivlheim_client
%config(noreplace) %{_sysconfdir}/nivlheim/client.conf

%files server
%defattr(-, root, root, -)
%config %{_sysconfdir}/httpd/conf.d/nivlheim.conf
%config %{_sysconfdir}/nivlheim/openssl_ca.conf
%config %{_sysconfdir}/systemd/system/%{name}.service
%{_localstatedir}/nivlheim/init.sql
%attr(0775, root, apache)
%dir /var/www/nivlheim
%attr(0775, root, apache)
%dir /var/log/nivlheim
/var/www/cgi-bin/ping
/var/www/cgi-bin/reqcert
/var/www/cgi-bin/processarchive
/var/www/cgi-bin/secure/ping
/var/www/cgi-bin/secure/renewcert
/var/www/cgi-bin/secure/post
%attr(0644, root, apache)
/var/www/nivlheim/log4perl.conf
%attr(0755, root, root)
%{_localstatedir}/nivlheim/nivlheim_setup.sh

%post server
%{_localstatedir}/nivlheim/nivlheim_setup.sh
%systemd_post %{name}.service

%preun server
%systemd_preun %{name}.service

%changelog
* Tue Jun  6 2017 Øyvind Hagberg <oyvind.hagberg@usit.uio.no> - 0.1.0
- First package build
