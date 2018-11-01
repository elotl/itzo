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
# aws s3 cp itzo s3://${s3_bucket}/ --acl public-read

echo "-1000" > /proc/self/oom_score_adj
itzo_dir=/usr/local/bin
\${itzo_dir}/cloud-init --from-metadata-service >> /var/log/itzo/itzo.log 2>&1

itzo_url_file="/tmp/milpa/itzo_url"
itzo_url="http://itzo-download.s3.amazonaws.com"
if [[ -f \$itzo_url_file ]]; then
    itzo_url=\$(head -n 1 \$itzo_url_file)
fi
itzo_version_file="/tmp/milpa/itzo_version"
itzo_version="latest"
if [[ -f \$itzo_version_file ]]; then
    itzo_version=\$(head -n 1 \$itzo_version_file)
fi
itzo_full_url="\${itzo_url}/itzo-\${itzo_version}"
itzo_path="\${itzo_dir}/itzo"
rm -f \$itzo_path
while true; do
    echo "\$(date) downloading itzo from \$itzo_full_url" >> /var/log/itzo/itzo_download.log 2>&1
    wget --timeout=3 \$itzo_full_url -O \$itzo_path && break >> /var/log/itzo/itzo_download.log 2>&1
    sleep 1
done

chmod 755 \$itzo_path
\${itzo_dir}/itzo >> /var/log/itzo/itzo.log 2>&1
EOF

chmod 755 /usr/local/bin/itzo_download.sh
cat /usr/local/bin/itzo_download.sh

step 'Add tosi'
wget -O /usr/local/bin/tosi http://tosi.s3.amazonaws.com/tosi
chmod 755 /usr/local/bin/tosi

step 'Add cloud-init'
wget -O /usr/local/bin/cloud-init http://itzo-download.s3.amazonaws.com/cloud-init
chmod 755 /usr/local/bin/cloud-init

step 'Add aws-ena module'
wget http://itzo-packages.s3.amazonaws.com/aws-ena-module.tar.gz
tar xvzf aws-ena-module.tar.gz
for kernel in /lib/modules/*; do
    mkdir -p "${kernel}/kernel/drivers/net/ethernet/amazon/ena/"
    cp ena.ko "${kernel}/kernel/drivers/net/ethernet/amazon/ena/"
    depmod -a "$(basename ${kernel})"
done
rm aws-ena-module.tar.gz
echo ena > /etc/modules-load.d/ena.conf

# Taken from https://github.com/mcrute/alpine-ec2-ami/blob/master/make_ami.sh
# Create ENA feature for mkinitfs
# Submitted upstream: https://github.com/alpinelinux/mkinitfs/pull/19
echo "kernel/drivers/net/ethernet/amazon/ena" > /etc/mkinitfs/features.d/ena.modules
# Enable ENA and NVME features these don't hurt for any instance and are
# hard requirements of the 5 series and i3 series of instances
sed -Ei 's/^features="([^"]+)"/features="\1 nvme ena"/' /etc/mkinitfs/mkinitfs.conf
/sbin/mkinitfs $(basename $(find /lib/modules/* -maxdepth 0))

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

cat > /etc/init.d/nvidia <<-EOF
#!/sbin/openrc-run

name=\$RC_SVCNAME

depend() {
        after bootmisc
        need localmount
}

start() {
  #
  # This is the recommended script from:
  #
  #   http://docs.nvidia.com/cuda/cuda-installation-guide-linux/index.html
  #
  /sbin/modprobe nvidia
  if [ "\$?" -eq 0 ]; then
    # Count the number of NVIDIA controllers found.
    NVDEVS=\`lspci | grep -i NVIDIA\`
    N3D=\`echo "\$NVDEVS" | grep "3D controller" | wc -l\`
    NVGA=\`echo "\$NVDEVS" | grep "VGA compatible controller" | wc -l\`
    N=\`expr \$N3D + \$NVGA - 1\`
    for i in \`seq 0 \$N\`; do
      if [[ ! -e "/dev/nvidia\$i" ]]; then
        echo "Creating /dev/nvidia\$i c 195 \$i"
        mknod -m 666 /dev/nvidia\$i c 195 \$i
      fi
    done
    if [[ ! -e "/dev/nvidiactl" ]]; then
      echo "Creating /dev/nvidiactl c 195 255"
      mknod -m 666 /dev/nvidiactl c 195 255
    fi
  fi

  /sbin/modprobe nvidia-uvm
  if [ "\$?" -eq 0 ]; then
    # Find out the major device number used by the nvidia-uvm driver
    D=\`grep nvidia-uvm /proc/devices | awk '{print \$1}'\`
    if [[ ! -e "/dev/nvidia-uvm" ]]; then
      echo "Creating /dev/nvidia-uvm c \$D 0"
      mknod -m 666 /dev/nvidia-uvm c \$D 0
    fi
  fi
}

stop() {
  for ko in nvidia-uvm nvidia-modeset nvidia-drm nvidia; do
    /bin/lsmod | grep "\<\$ko\>" && /sbin/rmmod \$ko
  done
}
EOF
chmod 755 /etc/init.d/nvidia
cat /etc/init.d/nvidia

step 'Load iptables modules at boot'
echo 'iptable_nat' >> /etc/modules

step 'automatically resize root partition'
cat > /etc/init.d/resizeroot <<-EOF
#!/sbin/openrc-run

name=\$RC_SVCNAME

depend() {
        need localmount
}

start() {
  rootdev=\$(mount -v | fgrep 'on / ' | cut -f 1 -d' ')
  /usr/sbin/resize2fs \$rootdev
}
EOF
chmod 755 /etc/init.d/resizeroot
cat /etc/init.d/resizeroot

step 'Enable vsyscall emulation'
sed -Ei -e "s|^[# ]*(default_kernel_opts)=.*|\1=\"vsyscall=emulate\"|" \
	/etc/update-extlinux.conf
update-extlinux --warn-only 2>&1 | grep -Fv 'extlinux: cannot open device /dev' >&2

step 'Enable services'
rc-update add acpid default
rc-update add chronyd default
rc-update add crond default
rc-update add net.eth0 default
rc-update add sshd default
rc-update add itzo default
rc-update add resizeroot default
rc-update add nvidia default
rc-update add net.lo boot
rc-update add termencoding boot
