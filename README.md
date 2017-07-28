# Getting started
### Install the server
1. Spin up a clean VM running Fedora 25, 26 or RHEL 7
2. Configure the yum repository:  
`/etc/yum.repos.d/nivlheim.repo`
```
[nivlheim]
name=Nivlheim
baseurl=http://folk.uio.no/oyvihag/nivlheimrepo
enabled=1
```
3. Install the packages:
```
sudo dnf -y --nogpgcheck install nivlheim-server nivlheim-client
```
4. Open the web admin interface in a browser:
`https://<your server>/`

At this point, there's no data in the system, because no clients have been configured yet.

### Get the client running

1. Edit `/etc/nivlheim/client.conf`, add one line:
```
server=localhost
```
2. The client package has already configured a cron job, but to speed things up you can manually run the client:
```
sudo /usr/sbin/nivlheim_client
```
3. Open the web admin interface in a browser, and you should see that there's a new machine waiting to be approved. Click "approve".

4. Run the client one more time:
```
sudo /usr/sbin/nivlheim_client
```

5. Wait a few seconds, and refresh the web page. You should see some information about the machine. It takes a few seconds for the system to process before it shows up.

### Install more clients

1. Spin up a new VM or use another existing machine.

2. Configure the yum repository as detailed above.

3. Install the `nivlheim-client` package.
```
sudo dnf -y --nogpgcheck install nivlheim-client
```

4. Edit `/etc/nivlheim/client.conf`, add one line with the server hostname or ip address
```
server=yourserver.example.com
```
5. If you are using a self-signed certificate for the web server (by default the nivlheim_server package will set it up with one), then the CA certificate file must be distributed to the clients.  
Copy `/var/www/nivlheim/CA/nivlheimca.crt` from the server, and place it in `/var/nivlheim` on the machine you're installing the client software on.

6. Run /usr/sbin/nivlheim_client manually (as root), or wait for cron to run it (could take up to one hour).

7. On the web admin pages the new machine will show up as waiting for approval. After it has been approved, and the client has run one more time, data from it will start showing up in the system.

# How to contribute
- Do you have a suggestion, feature request, or idea? Or have you found a bug? Go to the "issues" page and create a new issue! Everything is welcome.
- Would you like to contribute code? Fork the repository and create a pull request!
- Questions? Contact me at oyvind.hagberg@usit.uio.no
