FROM fedora:latest
MAINTAINER iti-dia@usit.uio.no
EXPOSE 80 443
LABEL no.uio.contact=usit-gid@usit.uio.no
LABEL description="Web server with CGI scripts for Nivlheim"

RUN dnf update -y
RUN dnf install -y httpd mod_ssl openssl perl procps-ng
RUN dnf clean all

# config
COPY server/openssl_ca.conf /etc/nivlheim/
COPY server/client_CA_cert.sh /usr/bin/
COPY server/httpd_ssl.conf /etc/httpd/conf.d/ssl.conf

# cgi scripts
COPY server/cgi/ping server/cgi/reqcert server/cgi/processarchive server/cgi/Database.pm /var/www/cgi-bin/
COPY server/cgi/ping2 /var/www/cgi-bin/secure/ping
COPY server/cgi/post server/cgi/renewcert /var/www/cgi-bin/secure/
RUN chmod -R a+x /var/www/cgi-bin/*

# entry point
COPY server/entrypoint.sh /usr/bin/
CMD /usr/bin/entrypoint.sh
