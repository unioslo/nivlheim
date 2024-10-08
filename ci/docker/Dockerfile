FROM fedora:latest
EXPOSE 80 443
LABEL no.uio.contact=usit-gid@usit.uio.no
LABEL description="Web server for Nivlheim"
ARG BRANCH=""

RUN dnf install -y httpd mod_ssl procps-ng \
		unzip file bind-utils npm \
	&& dnf install -y --releasever=39 openssl \
	&& dnf clean all \
	&& rm -rf /var/cache/yum \
	&& npm install -g handlebars

# config
COPY server/openssl_ca.conf /etc/nivlheim/
COPY server/client_CA_cert.sh /usr/bin/
COPY server/httpd_ssl.conf /etc/httpd/conf.d/ssl.conf
COPY server/httpd.conf /etc/httpd/conf/httpd.conf

# copy the static web content
COPY server/website /var/www/html/
RUN rm -rf /var/www/html/mockapi

# Compile web templates
RUN handlebars /var/www/html/templates -f /var/www/html/js/templates.js \
	&& rm -rf /var/www/html/templates

# download Javascript and CSS libraries
RUN /var/www/html/libs/download_libraries.sh

# entry point
COPY server/entrypoint.sh /usr/bin/
CMD /usr/bin/entrypoint.sh

# version
COPY VERSION /var/www/html/version.txt
RUN /bin/bash -c 'if [[ "${BRANCH}" != "" && "${BRANCH}" != "master" ]]; then echo "`cat /var/www/html/version.txt`-${BRANCH}" > /var/www/html/version.txt; fi'

# health check
HEALTHCHECK --start-period=10s CMD curl -kf https://localhost/cgi-bin/ping || exit 1
