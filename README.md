# Getting started

### Run a server
```
curl -s -O https://raw.githubusercontent.com/unioslo/nivlheim/master/docker-compose.yml
docker-compose up -d
```
At this point, there's no data in the system, because no clients have been configured yet.

### Run the client

This project used to provide rpms, but that function has been discontinued. The old rpm spec file is still included in the repo, in case anyone wants it.
It is up to you to decide how to distribute and run the client.

**Example:** How to use the client on Fedora 34:
1. Grab the code and the required libraries:
```
sudo dnf install -y perl openssl dmidecode \
	perl-Archive-Tar perl-File-Basename perl-File-Path perl-Getopt-Long \
	perl-HTTP-Message perl-IO perl-IO-Socket-INET6 perl-Net-DNS \
	perl-Sys-Hostname perl-Sys-Syslog perl-Socket
git clone https://github.com/unioslo/nivlheim.git
```
2. Create a config file
```
sudo mkdir /etc/nivlheim /var/nivlheim
sudo cp nivlheim/client/client.conf /etc/nivlheim
echo "server=localhost" | sudo tee -a /etc/nivlheim/client.conf
```
3. (Optional) Whitelist the IP range the client will be coming from:
```
curl -sS -X POST 'http://localhost:4040/api/v2/settings/ipranges' -d 'ipRange=172.16.0.0/12'
```
4. Run the client
```
sudo nivlheim/client/nivlheim_client --debug
```
5. (Optional) manually approve the client

If the IP address wasn't whitelisted, the server will require you to manually approve the new machine before data is processed.
On the web admin pages the new machine will show up as waiting for approval. After it has been approved, and the client has run one more time, data from it will start showing up in the system.

6. (Optional) Configure a cron job for the client:
```
sudo cp nivlheim/client/cronjob /etc/cron.d/
```

### Next steps

If you are using a self-signed certificate for the web server (by default the nivlheim server container will create one for itself), then the CA certificate file must be distributed to the clients.
Copy `/var/www/nivlheim/CA/nivlheimca.crt` from the server, and place it in `/var/nivlheim` on the machine you're installing the client software on.


# How to contribute
- Do you have a suggestion, feature request, or idea? Or have you found a bug? Go to the "issues" page and create a new issue! Everything is welcome.
- Would you like to contribute code? Fork the repository and create a pull request! We try to use the [GitHub workflow](https://guides.github.com/introduction/flow/). You can also ask to be added as a collaborator.
- Questions? Contact me at oyvind.hagberg@usit.uio.no
