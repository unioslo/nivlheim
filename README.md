# Getting started
### Install the server
1. Spin up a clean VM running Fedora 29, 28, RHEL 7, or CentOS 7
2. If you're using RHEL or CentOS, you need to [enable the EPEL package repository](https://fedoraproject.org/wiki/EPEL).
3. Configure the package repository:
```
sudo dnf copr enable oyvindh/Nivlheim
```
or go to [the project page at Fedora Copr](https://copr.fedorainfracloud.org/coprs/oyvindh/Nivlheim/),
download the appropriate repository config file, and place it in
`/etc/yum.repos.d/`  

4. Install the packages:
```
sudo dnf -y install nivlheim-server nivlheim-client
```
5. Open the web admin interface in a browser:
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

2. Configure the package repository as detailed above.

3. Install the `nivlheim-client` package.
```
sudo dnf -y install nivlheim-client
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
- Would you like to contribute code? Fork the repository and create a pull request! We try to use the [GitHub workflow](https://guides.github.com/introduction/flow/). You can also ask to be added as a collaborator.
- Questions? Contact me at oyvind.hagberg@usit.uio.no
