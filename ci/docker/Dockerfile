FROM alpine:3.18
EXPOSE 80 443
LABEL no.uio.contact=usit-dia@usit.uio.no
LABEL description="Web server for Nivlheim"
ARG BRANCH=""

RUN apk update && apk upgrade \
 	&& apk add apache2 apache2-ssl apache2-utils apache2-proxy procps-ng unzip file bind-tools npm bash curl \
 	&& apk add openssl1.1-compat && rm /usr/bin/openssl && ln /usr/bin/openssl1.1 /usr/bin/openssl \
 	&& rm -rf /var/cache/apk/*
RUN npm install -g handlebars

# config
COPY server/openssl_ca.conf /etc/nivlheim/
COPY server/client_CA_cert.sh /usr/bin/
COPY server/httpd_ssl.conf /etc/apache2/conf.d/ssl.conf
COPY server/httpd.conf /etc/apache2/httpd.conf

# before moving to Alping we used this log directory so let's continue with that
RUN mkdir /var/log/httpd

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
