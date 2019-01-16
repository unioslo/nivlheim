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
Source1:  https://github.com/lib/pq/archive/master.tar.gz#/pq-master.tar.gz
Source2:  https://github.com/golang/oauth2/archive/master.tar.gz#/oauth2-master.tar.gz
Source3:  https://github.com/golang/net/archive/master.tar.gz#/net-master.tar.gz
Source4:  https://github.com/jquery/jquery/archive/3.3.1.tar.gz
Source5:  https://cdnjs.cloudflare.com/ajax/libs/handlebars.js/4.0.12/handlebars.runtime.min.js
Source6:  https://github.com/moment/moment/archive/2.22.2.tar.gz
Source7:  https://github.com/jgthms/bulma/releases/download/0.7.2/bulma-0.7.2.zip
Source8:  https://github.com/CodeYellowBV/tarantino/archive/v2.1.0.tar.gz
Source9:  https://use.fontawesome.com/releases/v5.2.0/fontawesome-free-5.2.0-web.zip
Source10: https://raw.githubusercontent.com/wycats/handlebars.js/master/LICENSE

BuildRequires: npm(handlebars)
BuildRequires: perl(Archive::Tar)
BuildRequires: perl(Archive::Zip)
BuildRequires: perl(CGI)
BuildRequires: perl(Crypt::OpenSSL::X509)
BuildRequires: perl(DBD::Pg)
BuildRequires: perl(DBI)
BuildRequires: perl(Digest::CRC)
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
BuildRequires: perl(LWP::Simple)
BuildRequires: perl(MIME::Base64)
BuildRequires: perl(Net::CIDR)
BuildRequires: perl(Net::DNS)
BuildRequires: perl(Net::IP)
BuildRequires: perl(Proc::PID::File)
BuildRequires: perl(Socket)
BuildRequires: perl(Sys::Hostname)
BuildRequires: perl(Sys::Syslog)
BuildRequires: perl(Time::Piece)
BuildRequires: systemd, golang, git

%description
This package is the base package for Nivlheim.

%package client
Summary:  Client component of Nivlheim
Group:    Applications/System
BuildArch: noarch
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
Requires: perl(Sys::Hostname)
Requires: perl(Sys::Syslog)

%package server
Summary:  Server components of Nivlheim
Group:    Applications/System
Requires: perl, openssl, httpd, mod_ssl, systemd
Requires: postgresql, postgresql-server, postgresql-contrib
Requires: unzip, file
Requires: perl(Archive::Tar)
Requires: perl(Archive::Zip)
Requires: perl(CGI)
Requires: perl(Crypt::OpenSSL::X509)
Requires: perl(DBD::Pg)
Requires: perl(DBI)
Requires: perl(Digest::CRC)
Requires: perl(Encode)
Requires: perl(File::Basename)
Requires: perl(File::Copy)
Requires: perl(File::Find)
Requires: perl(File::Temp)
Requires: perl(JSON)
Requires: perl(Log::Log4perl)
Requires: perl(Log::Log4perl::Level)
Requires: perl(Log::Dispatch)
Requires: perl(Log::Dispatch::FileRotate)
Requires: perl(Log::Dispatch::Syslog)
Requires: perl(LWP::Simple)
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
%setup -q -T -b 1 -n pq-master
%setup -q -T -b 2 -n oauth2-master
%setup -q -T -b 3 -n net-master
%setup -q -T -b 4 -n jquery-3.3.1
%setup -q -T -b 6 -n moment-2.22.2
%setup -q -T -b 8 -n tarantino-2.1.0
cd %{_builddir}
rm -rf bulma-0.7.2 fontawesome-free-5.2.0-web
unzip -q %{SOURCE7}
unzip -q %{SOURCE9}
mkdir -p fontawesome/css
mv fontawesome-free-5.2.0-web/webfonts fontawesome/
mv fontawesome-free-5.2.0-web/css/all.css fontawesome/css/
mv fontawesome-free-5.2.0-web/LICENSE.txt fontawesome/
chmod -R a+rX,g-w,o-w bulma-0.7.2 fontawesome
%autosetup -D -n %{name}-%{getenv:GIT_BRANCH}

# disable building of the debug package.
# avoids the error debuginfo-without-sources from rpmlint
%define debug_package %{nil}

%build
# Compile web templates
handlebars server/website/templates --min -f server/website/js/templates.js
# Compile system service
export GOPATH=`pwd`/gopath
rm -rf $GOPATH
mkdir -p $GOPATH/{src,bin}
export GOBIN="$GOPATH/bin"
mkdir -p $GOPATH/src/github.com/usit-gd/nivlheim
mv server $GOPATH/src/github.com/usit-gd/nivlheim
mkdir -p $GOPATH/src/github.com/lib/
mv ../pq-master $GOPATH/src/github.com/lib/pq
mkdir -p $GOPATH/src/golang.org/x
mv ../net-master $GOPATH/src/golang.org/x/net
mv ../oauth2-master $GOPATH/src/golang.org/x/oauth2
pushd $GOPATH/src/github.com/usit-gd/nivlheim/server/service
NONETWORK=1 NOPOSTGRES=1 go test -v
rm -f $GOPATH/bin/*
# The linkmode=external flag is a fix for the error "No build ID note found in ...".
# The -w flag disables debug info generation, and -s omits the symbol table.
# By using -w and -s we avoid the unstripped-binary-or-object warning from rpmlint
go install -ldflags='-linkmode=external -w -s'
popd
mv $GOPATH/src/github.com/usit-gd/nivlheim/server .

%install
rm -rf %{buildroot}
mkdir -p %{buildroot}%{_sbindir}
mkdir -p %{buildroot}%{_sysconfdir}/nivlheim
mkdir -p %{buildroot}%{_sysconfdir}/httpd/conf.d
mkdir -p %{buildroot}%{_localstatedir}/nivlheim
mkdir -p %{buildroot}/var/www/nivlheim
mkdir -p %{buildroot}/var/www/cgi-bin/secure
mkdir -p %{buildroot}/var/www/html/libs
mkdir -p %{buildroot}/var/log/nivlheim
mkdir -p %{buildroot}%{_unitdir}
install -p -m 0755 client/nivlheim_client %{buildroot}%{_sbindir}/
install -p -m 0644 client/client.conf %{buildroot}%{_sysconfdir}/nivlheim/
install -p -m 0644 server/httpd_ssl.conf %{buildroot}%{_sysconfdir}/httpd/conf.d/nivlheim.conf
install -p -m 0644 server/openssl_ca.conf %{buildroot}%{_sysconfdir}/nivlheim/
install -p -m 0644 server/server.conf %{buildroot}%{_sysconfdir}/nivlheim/
install -p -m 0755 server/cgi/ping %{buildroot}/var/www/cgi-bin/
install -p -m 0755 server/cgi/ping2 %{buildroot}/var/www/cgi-bin/secure/ping
install -p -m 0755 server/cgi/reqcert %{buildroot}/var/www/cgi-bin/
install -p -m 0755 server/cgi/renewcert %{buildroot}/var/www/cgi-bin/secure/
install -p -m 0755 server/cgi/post %{buildroot}/var/www/cgi-bin/secure/
install -p -m 0644 server/log4perl.conf %{buildroot}/var/www/nivlheim/
install -p -m 0755 server/setup.sh %{buildroot}%{_localstatedir}/nivlheim/
install -p -m 0755 server/cgi/processarchive %{buildroot}/var/www/cgi-bin/
install -p -m 0644 server/nivlheim.service %{buildroot}%{_unitdir}/%{name}.service
install -p -m 0644 -D client/cronjob %{buildroot}%{_sysconfdir}/cron.d/nivlheim_client
rm -rf server/website/mockapi server/website/templates server/website/libs
cp -a server/website/* %{buildroot}%{_localstatedir}/www/html/
install -p -m 0644 ../jquery-3.3.1/dist/jquery.min.js %{buildroot}%{_localstatedir}/www/html/libs/jquery-3.3.1.min.js
install -p -m 0644 ../jquery-3.3.1/LICENSE.txt %{buildroot}%{_localstatedir}/www/html/libs/jquery-license.txt
install -p -m 0644 %{SOURCE5} %{buildroot}%{_localstatedir}/www/html/libs/handlebars.min.js
install -p -m 0644 %{SOURCE10} %{buildroot}%{_localstatedir}/www/html/libs/handlebars-license.txt
install -p -m 0644 ../moment-2.22.2/min/moment.min.js %{buildroot}%{_localstatedir}/www/html/libs/
install -p -m 0644 ../moment-2.22.2/LICENSE %{buildroot}%{_localstatedir}/www/html/libs/moment-license.txt
install -p -m 0644 ../bulma-0.7.2/css/bulma.min.css %{buildroot}%{_localstatedir}/www/html/libs/
install -p -m 0644 ../bulma-0.7.2/LICENSE %{buildroot}%{_localstatedir}/www/html/libs/bulma-license.txt
install -p -m 0644 ../tarantino-2.1.0/build/tarantino.min.js %{buildroot}%{_localstatedir}/www/html/libs/
install -p -m 0644 ../tarantino-2.1.0/LICENSE %{buildroot}%{_localstatedir}/www/html/libs/tarantino-license.txt
cp -a ../fontawesome %{buildroot}%{_localstatedir}/www/html/libs
chmod 755 %{buildroot}%{_localstatedir}/www/html/libs
install -p -m 0755 gopath/bin/service %{buildroot}%{_sbindir}/nivlheim_service
cp -a server/database/* %{buildroot}%{_localstatedir}/nivlheim/
echo %{version} > %{buildroot}%{_sysconfdir}/nivlheim/version
echo %{version} > %{buildroot}%{_localstatedir}/www/html/version.txt
# add the version number to js and css urls to ensure browsers will reload
sed -i 's/src="\(.\+.js\)"/src="\1?%{version}"/g' %{buildroot}%{_localstatedir}/www/html/index.html
sed -i 's/href="\(.\+.js\)"/href="\1?%{version}"/g' %{buildroot}%{_localstatedir}/www/html/index.html

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

%files client
%defattr(-, root, root, -)
%dir %{_localstatedir}/nivlheim
%dir %{_sysconfdir}/nivlheim
%license LICENSE.txt
%doc README.md
%{_sbindir}/nivlheim_client
%config(noreplace) %{_sysconfdir}/nivlheim/client.conf
%config %{_sysconfdir}/cron.d/nivlheim_client

%files server
%defattr(-, root, root, -)
%{_localstatedir}/nivlheim
%dir %{_sysconfdir}/nivlheim
%license LICENSE.txt
%doc README.md
%config %{_sysconfdir}/nivlheim/version
%config(noreplace) %{_sysconfdir}/httpd/conf.d/nivlheim.conf
%config %{_sysconfdir}/nivlheim/openssl_ca.conf
%config(noreplace) %{_sysconfdir}/nivlheim/server.conf
%{_unitdir}/%{name}.service
%{_sbindir}/nivlheim_service
%dir /var/log/nivlheim
/var/www/cgi-bin/*
/var/www/html/*
%attr(0644, root, apache) /var/www/nivlheim/log4perl.conf
%attr(0755, root, root) %{_localstatedir}/nivlheim/setup.sh

%post server
%{_localstatedir}/nivlheim/setup.sh || exit 1
%systemd_post %{name}.service

%preun server
%systemd_preun %{name}.service

%postun server
%systemd_postun_with_restart %{name}.service

%changelog
* Tue Dec 11 2018 Øyvind Hagberg <oyvind.hagberg@usit.uio.no> - 0.11.0-20181211
- Include 3rd party javascript and css libraries in the rpm file

* Tue Aug  7 2018 Øyvind Hagberg <oyvind.hagberg@usit.uio.no> - 0.9.0-20180807
- Added sources for Go package golang.org/x/oauth2 and its dependencies

* Tue May  1 2018 Øyvind Hagberg <oyvind.hagberg@usit.uio.no> - 0.6.1-20180501
- Replaced init.sql with a set of sql patch files and an install script

* Wed Apr 18 2018 Øyvind Hagberg <oyvind.hagberg@usit.uio.no> - 0.6.0-20180418
- The client requires perl(Sys::Hostname), and has a new cron job

* Tue Mar 27 2018 Øyvind Hagberg <oyvind.hagberg@usit.uio.no> - 0.4.0-20180327
- Removed the cgi script "parsefile"

* Tue Feb 27 2018 Øyvind Hagberg <oyvind.hagberg@usit.uio.no> - 0.2.0-20180227
- Compile Go code during build, distribute binaries instead of source.

* Fri Feb 23 2018 Øyvind Hagberg <oyvind.hagberg@usit.uio.no> - 0.1.4-20180223
- New web frontend, installs in /var/www/html. frontpage.cgi is gone.

* Fri Jan 05 2018 Øyvind Hagberg <oyvind.hagberg@usit.uio.no> - 0.1.1-20180105
- Removed dependencies on the missing parent package "nivlheim",
  since it isn't built anymore.

* Wed Jan 03 2018 Øyvind Hagberg <oyvind.hagberg@usit.uio.no> - 0.1.1-20180103
- Removed use of GIT_URL macro. Removed faulty %%attr directives.

* Thu Sep 14 2017 Øyvind Hagberg <oyvind.hagberg@usit.uio.no> - 0.1.0-20170914
- Use macros for Source0 and URL. Values come from Jenkins

* Fri Jul 21 2017 Øyvind Hagberg <oyvind.hagberg@usit.uio.no> - 0.1.0-20170721
- First package build
