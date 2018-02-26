%global date %(date +"%Y%m%d%H%M")

# Semantic Versioning http://semver.org/
Name:     nivlheim
Version:  %{getenv:GIT_TAG}
Release:  %{date}%{?dist}

Summary:  File collector

Group:    Applications/System
License:  GPLv3+

URL:      https://github.com/usit-gd/nivlheim
Source0:  https://github.com/usit-gd/nivlheim/archive/%{getenv:GIT_BRANCH}.tar.gz

BuildRequires: npm(handlebars)
BuildRequires: perl(Archive::Tar)
BuildRequires: perl(Archive::Zip)
BuildRequires: perl(CGI)
BuildRequires: perl(Crypt::OpenSSL::X509)
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
BuildRequires: perl(JSON)
BuildRequires: perl(Log::Log4perl)
BuildRequires: perl(Log::Log4perl::Level)
BuildRequires: perl(MIME::Base64)
BuildRequires: perl(Net::CIDR)
BuildRequires: perl(Net::DNS)
BuildRequires: perl(Net::IP)
BuildRequires: perl(Proc::PID::File)
BuildRequires: perl(Socket)
BuildRequires: perl(Sys::Syslog)
BuildRequires: perl(Time::Piece)
BuildRequires: systemd

%global _binary_filedigest_algorithm 1
%global _source_filedigest_algorithm 1
BuildArch: noarch

%description
This package is the base package for Nivlheim.

%package client
Summary:  Client component of Nivlheim
Group:    Applications/System
Requires: perl, openssl, dmidecode
Requires: perl(Archive::Tar)
Requires: perl(File::Basename)
Requires: perl(File::Path)
Requires: perl(Getopt::Long)
Requires: perl(HTTP::Request::Common)
Requires: perl(IO::File)
Requires: perl(IO::Socket::INET6)
Requires: perl(IO::Socket::SSL)
Requires: perl(Net::DNS)
Requires: perl(Socket)
Requires: perl(Sys::Syslog)

%package server
Summary:  Server components of Nivlheim
Group:    Applications/System
Requires: perl, openssl, httpd, mod_ssl, postgresql, postgresql-server
Requires: golang, unzip, file, git
Requires: perl(Archive::Tar)
Requires: perl(Archive::Zip)
Requires: perl(CGI)
Requires: perl(Crypt::OpenSSL::X509)
Requires: perl(DBD::Pg)
Requires: perl(DBI)
Requires: perl(Encode)
Requires: perl(File::Basename)
Requires: perl(File::Copy)
Requires: perl(File::Find)
Requires: perl(File::Temp)
Requires: perl(JSON)
Requires: perl(Log::Log4perl)
Requires: perl(Log::Log4perl::Level)
Requires: perl(MIME::Base64)
Requires: perl(Net::CIDR)
Requires: perl(Net::DNS)
Requires: perl(Net::IP)
Requires: perl(Proc::PID::File)
Requires: perl(Time::Piece)

%description client
This package contains the client component of Nivlheim.

%description server
This package contains the server components of Nivlheim.

%prep
%autosetup -n %{name}-%{getenv:GIT_BRANCH}

%build

%install
rm -rf %{buildroot}
mkdir -p %{buildroot}%{_sbindir}
mkdir -p %{buildroot}%{_sysconfdir}/nivlheim
mkdir -p %{buildroot}%{_sysconfdir}/httpd/conf.d
mkdir -p %{buildroot}%{_localstatedir}/nivlheim/go/{src,pkg,bin}
mkdir -p %{buildroot}/var/www/nivlheim
mkdir -p %{buildroot}/var/www/cgi-bin/secure
mkdir -p %{buildroot}/var/www/html
mkdir -p %{buildroot}/var/log/nivlheim
mkdir -p %{buildroot}%{_unitdir}
mkdir -p %{buildroot}%{_sysconfdir}/logrotate.d
install -p -m 0755 client/nivlheim_client %{buildroot}%{_sbindir}/
install -p -m 0644 client/client.conf %{buildroot}%{_sysconfdir}/nivlheim/
install -p -m 0644 server/httpd_ssl.conf %{buildroot}%{_sysconfdir}/httpd/conf.d/nivlheim.conf
install -p -m 0644 server/openssl_ca.conf %{buildroot}%{_sysconfdir}/nivlheim/
install -p -m 0755 server/cgi/ping %{buildroot}/var/www/cgi-bin/
install -p -m 0755 server/cgi/ping2 %{buildroot}/var/www/cgi-bin/secure/ping
install -p -m 0755 server/cgi/reqcert %{buildroot}/var/www/cgi-bin/
install -p -m 0755 server/cgi/renewcert %{buildroot}/var/www/cgi-bin/secure/
install -p -m 0755 server/cgi/post %{buildroot}/var/www/cgi-bin/secure/
install -p -m 0644 server/log4perl.conf %{buildroot}/var/www/nivlheim/
install -p -m 0755 server/setup.sh %{buildroot}%{_localstatedir}/nivlheim/
install -p -m 0644 server/init.sql %{buildroot}%{_localstatedir}/nivlheim/
install -p -m 0755 server/cgi/processarchive %{buildroot}/var/www/cgi-bin/
install -p -m 0755 server/cgi/parsefile %{buildroot}/var/www/cgi-bin/
install -p -m 0644 server/nivlheim.service %{buildroot}%{_unitdir}/%{name}.service
install -p -m 0644 server/logrotate.conf %{buildroot}%{_sysconfdir}/logrotate.d/%{name}-server
install -p -m 0755 -D client/cron_hourly %{buildroot}%{_sysconfdir}/cron.hourly/nivlheim_client
cp -r server/service %{buildroot}%{_localstatedir}/nivlheim/go/src/
echo %{version} > %{buildroot}%{_sysconfdir}/nivlheim/version

# Compile web templates
handlebars server/website/templates --min -f server/website/js/templates.js

# Copy static website files, excluding files that are only for development
rm -rf server/website/mockapi server/website/templates
cp -r server/website/* %{buildroot}%{_localstatedir}/www/html/

%check
perl -c %{buildroot}%{_sbindir}/nivlheim_client
perl -c %{buildroot}/var/www/cgi-bin/secure/renewcert
perl -c %{buildroot}/var/www/cgi-bin/secure/ping
perl -c %{buildroot}/var/www/cgi-bin/secure/post
perl -c %{buildroot}/var/www/cgi-bin/ping
perl -c %{buildroot}/var/www/cgi-bin/reqcert
perl -c %{buildroot}/var/www/cgi-bin/processarchive
perl -c %{buildroot}/var/www/cgi-bin/parsefile

%clean
rm -rf %{buildroot}

%files client
%defattr(-, root, root, -)
%dir %{_localstatedir}/nivlheim
%dir %{_sysconfdir}/nivlheim
%license LICENSE.txt
%doc README.md
%{_sbindir}/nivlheim_client
%config %{_sysconfdir}/nivlheim/version
%config(noreplace) %{_sysconfdir}/nivlheim/client.conf
%{_sysconfdir}/cron.hourly/nivlheim_client

%files server
%defattr(-, root, root, -)
%dir %{_localstatedir}/nivlheim
%dir %{_sysconfdir}/nivlheim
%license LICENSE.txt
%doc README.md
%config %{_sysconfdir}/nivlheim/version
%config %{_sysconfdir}/httpd/conf.d/nivlheim.conf
%config %{_sysconfdir}/nivlheim/openssl_ca.conf
%config %{_sysconfdir}/logrotate.d/%{name}-server
%{_unitdir}/%{name}.service
%{_localstatedir}/nivlheim/init.sql
%dir /var/log/nivlheim
/var/www/nivlheim
/var/www/cgi-bin
/var/www/html/*
%attr(0644, root, apache) /var/www/nivlheim/log4perl.conf
%attr(0755, root, root) %{_localstatedir}/nivlheim/setup.sh
%{_localstatedir}/nivlheim

%post server
%{_localstatedir}/nivlheim/setup.sh || exit 1
%systemd_post %{name}.service

%preun server
%systemd_preun %{name}.service

%postun server
%systemd_postun_with_restart %{name}.service

%changelog
* Fri Feb 23 2018 Øyvind Hagberg <oyvind.hagberg@usit.uio.no> - 0.1.4-20180223
- New web frontend, installs in /var/www/html. frontpage.cgi is gone.

* Fri Jan 05 2018 Øyvind Hagberg <oyvind.hagberg@usit.uio.no> - 0.1.1-20180105
- Removed dependencies on the missing parent package "nivlheim",
  since it isn't being build anymore.

* Wed Jan 03 2018 Øyvind Hagberg <oyvind.hagberg@usit.uio.no> - 0.1.1-20180103
- Removed use of GIT_URL macro. Removed faulty %%attr directives.

* Thu Sep 14 2017 Øyvind Hagberg <oyvind.hagberg@usit.uio.no> - 0.1.0-20170914
- Use macros for Source0 and URL. Values come from Jenkins

* Fri Jul 21 2017 Øyvind Hagberg <oyvind.hagberg@usit.uio.no> - 0.1.0-20170721
- First package build
