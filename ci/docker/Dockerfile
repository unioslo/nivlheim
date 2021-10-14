FROM fedora:latest
MAINTAINER iti-dia@usit.uio.no
EXPOSE 80 443
LABEL no.uio.contact=usit-gid@usit.uio.no
LABEL description="Web server with CGI scripts for Nivlheim"

RUN dnf install -y httpd mod_ssl openssl perl procps-ng unzip file bind-utils \
		perl-Archive-Tar perl-Archive-Zip perl-CGI perl-Crypt-OpenSSL-X509 \
		perl-DBD-Pg perl-DBI perl-Digest-CRC perl-Encode \
		perl-File-Basename perl-File-Path perl-File-Temp \
		perl-JSON perl-Log-Log4perl perl-Log-Dispatch perl-Log-Dispatch-FileRotate \
		perl-Net-CIDR perl-Net-DNS perl-Net-IP \
		perl-Proc-PID-File perl-Time-Piece \
	&& dnf clean all \
	&& rm -rf /var/cache/yum

# config
COPY server/openssl_ca.conf server/log4perl.conf /etc/nivlheim/
COPY server/client_CA_cert.sh /usr/bin/
COPY server/httpd_ssl.conf /etc/httpd/conf.d/ssl.conf

# cgi scripts
COPY server/cgi/ping server/cgi/reqcert server/cgi/processarchive /var/www/cgi-bin/
COPY server/cgi/ping2 /var/www/cgi-bin/secure/ping
COPY server/cgi/post server/cgi/renewcert /var/www/cgi-bin/secure/
COPY server/cgi/Database.pm /usr/lib64/perl5/Nivlheim/
RUN chmod -R a+x /var/www/cgi-bin/*

# entry point
COPY server/entrypoint.sh /usr/bin/
CMD /usr/bin/entrypoint.sh

# health check
HEALTHCHECK --start-period=10s CMD curl -kf https://localhost/cgi-bin/ping || exit 1
