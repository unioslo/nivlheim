# Getting started
### Install the server
1. Spin up a clean VM running Fedora 25, 26 or RHEL 7
1. Configure the yum repository:  
`/etc/yum.repos.d/nivlheim.repo`
```
[nivlheim]
name=Nivlheim
baseurl=http://folk.uio.no/oyvihag/nivlheimrepo
enabled=1
```
1. Install the packages:
```
sudo dnf -y --nogpgcheck install nivlheim-server nivlheim-client
```
1. Open the web admin interface in a browser:
`https://<your server>/`

At this point, there's no data in the system, because no clients have been configured yet.

### Get the client running

1. Edit `/etc/nivlheim/client.conf`, add one line:
```
server=localhost
```
1. The client package has already configured a cron job, but to speed things up you can manually run the client:
```
sudo /usr/sbin/nivlheim_client
```
1. Open web admin pages in a browser, and you should see that there's a new machine waiting to be approved. Click "approve"

1. Run the client one more time:
```
sudo /usr/sbin/nivlheim_client
```

1. Refresh the web pages and you should see some information about the machine. It may take a few seconds for the system to process before it shows up.

### Install more clients

1. Spin up a new VM or use another existing machine

1. Configure the yum repository as detailed above

1. Install the `nivlheim_client` package

1. Edit `/etc/nivlheim/client.conf`, add one line with the server address

1. If you are using a self-signed certificate for the web server (by default the nivlheim_server package will set it up with one), then the CA certificate file must be distributed to the clients.  
Copy `/var/www/nivlheim/CA/nivlheimca.crt` from the server, and place it in `/var/nivlheim` on the machine you're installing the client software on.

1. Optionally, run /usr/sbin/nivlheim_client manually, or wait for cron to run it (could take up to one hour)

1. On the web admin pages the new machine will show up as waiting for approval. After it has been approved, data from it will start showing  up in the system.
