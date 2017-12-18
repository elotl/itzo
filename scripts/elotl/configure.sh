#!/bin/sh

_step_counter=0
step() {
	_step_counter=$(( _step_counter + 1 ))
	printf '\n\033[1;36m%d) %s\033[0m\n' $_step_counter "$@" >&2  # bold cyan
}


step 'Set up timezone'
setup-timezone -z US/Pacific

step 'Set up networking'
cat > /etc/network/interfaces <<-EOF
	iface lo inet loopback
	iface eth0 inet dhcp
EOF
ln -s networking /etc/init.d/net.lo
ln -s networking /etc/init.d/net.eth0

step 'Adjust rc.conf'
sed -Ei \
	-e 's/^[# ](rc_depend_strict)=.*/\1=NO/' \
	-e 's/^[# ](rc_logger)=.*/\1=YES/' \
	-e 's/^[# ](rc_verbose)=.*/\1=YES/' \
	-e 's/^[# ](unicode)=.*/\1=YES/' \
	/etc/rc.conf

step 'Create ld-linux-x86-64.so.2 link'
mkdir -p /lib64
ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2

step 'Install ssh authorized keys'
mkdir -p /root/.ssh
chmod 0700 /root/.ssh
cat > /root/.ssh/authorized_keys <<-EOF
ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQDPU7h8CaYA1VH/CwY3Ahw0s0wPbB/8t7A96GX/6qS2a8n79nGjywThZJP15L7NnTdTrdV59NEC1QvS0ym/JhlpokXwgMWURsYOAP5Y2lmK7wvAZ65bDn0iPiXXgyWPtEWQqCTV0U9HfZ81m+JMzfcED+L3w0iZAHSeRlupPZRtea3izx91A19RRn0NyVtmrwF4h3g537p+0O3DvaktxZddnwa3vPbY3CE6Eijiqsy9HOrx49YJS3SdBMvGNx91pynVLPWTCziBmYZCt8ioTGNvF8YWLVRf6VCj6M9zTG2NkCbXydAxpfRByTa+4yyKE44hmAehDM15pQGlmcg0O4HlepTqOvPZVyWAvkO3aD1xycrWSTKu68IgRzm9Ve064h3OUqVcWx1tybEAGyioC/H/vdJ4BGKH1wfQQvRbWrO8gCAr8LGS8JUIWWDPOCBtFobsyMo2opck9t8iM8lAiscueMNTJeRuIeK6692m0OsXL9+g8lHJkTD97VF963liCeRhaIG3kIaYXTyOhQdKbDQShT/r4yC8eMWDR/I6ab+2ir/qew46XwHJ98c/Ux0zII5v252D5Q/A4Wf6HJGOjAoMx4iQJ8Q5LYpLnIcX1WqznJQx1zPpaI9WpFe4ELK/mBAv53Emp3HjfacI74/RM6Zt/EHYcKV3Cr5VMaxgLaReaw== bcox@elotl.co
ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQC/ywpZThNo+8665ftf8Se5hDfPewxsqvUiT2MifeBSHCHf0VSWKKDa9BG0o/gKVjEYsxrHBzR14SsPbhn2khP1jTKU1JPrSymqv2Zt5StGwadAWwXMn748zAu7mnIbioNn5pZ70K2eq/Q4LskQe0Sl9xpj92sPfLTEt3eAsPolIgvacy+qmlGmF7Zm+sFmmj+AvFyEFlhbsv8s92iwlGCNMXO2BkRTzYsTaLUVcf+xXbdxrxhTp15TnxzbMvBYTGSCnnCzK619zwDlY4MK33QWr3tEGVZalFXFX1NqRZbo1W8mfVHCG4bP/nMZZrROMTPmjl/KsMTC+4xRdzBlUO+7Fwd0qBEWyMe/90dtZajvXE20quLj7Dbbbx1UVuAXQXeiw5vTwPcwTBFqrbJ9dTyUqedQ2I/H8UeeQFkpTn3BC5yuccbpnVJg0yOBB4F4nUUdBSXCTTyz6I4EsBjhz12pyhGPljTgkcXJWmiv5bybD0EQQ+zZ4a077Ev9HTYo65xsCHr+Vh6KEy5TykV71lePgnfKpFTcsR5okuN9me8wRVpuytAZevnDwUhYblpBOl4GJo1yTZeQ+vPWKM3FJ7DvBY8w4TOdYsubcZwLcVIxEfMwTntohOOtATDQs8UlvLvhLPqtACDkpdHokb2YoDZFnewXwplReoe3hw7XlOb3cw== vilmos@x1
ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCLoV/djnLOJt7gRprssm36m3kn2kwSYbjyh93A2/oHNcPM1rvONdeKQ2qAK/DbRVjGIIdcskDQWCAxTOsN4OEBkIOHsWlfE3TJ8f+IqGYyG1Ly+I66GIulWqZQfRb8H+4xKxVZ9C5g8eM58tYOAsToDFa/IX7RX2Ayu0BNc92+cUV8w92mL2c76MCwbKviipK7T6lDVn6IINxtJHVHwFZYonIiBYKn/30zGrL17sMbOVVd27lYPI/To61ui5PFSym7d/BOEkKRQKRgnHfpwm+VhXUuOGCv1FIeBZSs95JVhbIzynld8NBY3SjIGWi3m22HXR8yYlXRNZwB+BSrcWKL madhuri@elotl.co
EOF
chmod 0600 /root/.ssh/authorized_keys
cat /root/.ssh/authorized_keys

step 'Enable ssh login for root'
echo "PermitRootLogin yes" >> /etc/ssh/sshd_config
cat /etc/ssh/sshd_config

PW="$(cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w 32 | head -n 1)"
step "Setting root password to '$PW'"
echo -en "$PW\n$PW\n" | passwd root

step 'Create itzo init script'
cat > /etc/init.d/itzo <<-EOF
#!/sbin/openrc-run

name=\$RC_SVCNAME
# cfgfile="/etc/\$RC_SVCNAME/\$RC_SVCNAME.conf"
command="/usr/local/bin/itzo_download.sh"
command_args=""
command_user="root"
pidfile="/run/\$RC_SVCNAME/\$RC_SVCNAME.pid"
start_stop_daemon_args=""
command_background="yes"

depend() {
        need net localmount
}

start_pre() {
        checkpath --directory --owner \$command_user:\$command_user --mode 0775 \
                /run/\$RC_SVCNAME /var/log/\$RC_SVCNAME
}
EOF
chmod 755 /etc/init.d/itzo
cat /etc/init.d/itzo

step 'Add itzo downloader-launcher'
cat > /usr/local/bin/itzo_download.sh <<-EOF
#!/bin/sh

# upload file with cli:
# aws s3 cp itzo s3://itzo-download/ --acl public-read

s3_path="http://itzo-download.s3.amazonaws.com/itzo"
itzo_dir=/usr/local/bin
itzo_path=\${itzo_dir}/itzo
rm -f \$itzo_path
while true; do
	echo "$(date) downloading itzo from S3" >> /var/log/itzo/itzo_download.log 2>&1
	wget --timeout=3 \$s3_path -P \$itzo_dir && break >> /var/log/itzo/itzo_download.log 2>&1
done
chmod 755 \$itzo_path
\$itzo_path >> /var/log/itzo/itzo.log 2>&1
EOF
chmod 755 /usr/local/bin/itzo_download.sh
cat /usr/local/bin/itzo_download.sh

#
# Note: the driver and libcuda client libraries need to be in sync, e.g. both
# using the 387.26 interface.
#
step 'Add NVidia driver'
wget http://itzo-packages.s3.amazonaws.com/nvidia.tar.gz
tar xvzf nvidia.tar.gz
for kernel in /lib/modules/*; do
    mkdir -p "${kernel}/misc"
    cp nvidia*.ko "${kernel}/misc/"
    depmod -a "$(basename ${kernel})"
done
rm nvidia.tar.gz

cat > /etc/modprobe.d/nvidia.conf <<-EOF
blacklist amd76x_edac
blacklist vga16fb
blacklist nouveau
blacklist rivafb
blacklist nvidiafb
blacklist rivatv
EOF

step 'Enable services'
rc-update add acpid default
rc-update add chronyd default
rc-update add crond default
rc-update add net.eth0 default
rc-update add sshd default
rc-update add itzo default
rc-update add net.lo boot
rc-update add termencoding boot
