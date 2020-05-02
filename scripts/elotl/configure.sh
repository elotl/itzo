#!/bin/sh

# Copyright 2020 Elotl Inc
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# At this time, this script takes only 1 argument: what cloud we're running on
CLOUD_PROVIDER=$1

_step_counter=0
step() {
	_step_counter=$(( _step_counter + 1 ))
	printf '\n\033[1;36m%d) %s\033[0m\n' $_step_counter "$@" >&2  # bold cyan
}


echo "Running configure.sh for cloud provider $CLOUD_PROVIDER"

step "Update packages"
apk update && apk upgrade

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


step 'Set password for root'
PW="$(cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w 32 | head -n 1)"
step "Setting root password to '$PW'"
echo -en "$PW\n$PW\n" | passwd root

# step "add bcox key to authorized keys"
# mkdir -p /root/.ssh
# chmod 0700 /root/.ssh
# cat > /root/.ssh/authorized_keys <<-EOF
# ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQDPU7h8CaYA1VH/CwY3Ahw0s0wPbB/8t7A96GX/6qS2a8n79nGjywThZJP15L7NnTdTrdV59NEC1QvS0ym/JhlpokXwgMWURsYOAP5Y2lmK7wvAZ65bDn0iPiXXgyWPtEWQqCTV0U9HfZ81m+JMzfcED+L3w0iZAHSeRlupPZRtea3izx91A19RRn0NyVtmrwF4h3g537p+0O3DvaktxZddnwa3vPbY3CE6Eijiqsy9HOrx49YJS3SdBMvGNx91pynVLPWTCziBmYZCt8ioTGNvF8YWLVRf6VCj6M9zTG2NkCbXydAxpfRByTa+4yyKE44hmAehDM15pQGlmcg0O4HlepTqOvPZVyWAvkO3aD1xycrWSTKu68IgRzm9Ve064h3OUqVcWx1tybEAGyioC/H/vdJ4BGKH1wfQQvRbWrO8gCAr8LGS8JUIWWDPOCBtFobsyMo2opck9t8iM8lAiscueMNTJeRuIeK6692m0OsXL9+g8lHJkTD97VF963liCeRhaIG3kIaYXTyOhQdKbDQShT/r4yC8eMWDR/I6ab+2ir/qew46XwHJ98c/Ux0zII5v252D5Q/A4Wf6HJGOjAoMx4iQJ8Q5LYpLnIcX1WqznJQx1zPpaI9WpFe4ELK/mBAv53Emp3HjfacI74/RM6Zt/EHYcKV3Cr5VMaxgLaReaw== bcox@elotl.co
# EOF
# chmod 0600 /root/.ssh/authorized_keys
# cat /root/.ssh/authorized_keys

# step 'Enable ssh login for root'
# echo "PermitRootLogin yes" >> /etc/ssh/sshd_config
# cat /etc/ssh/sshd_config

step 'add bcox'
adduser --disabled-password --gecos "" --home /home/bcox bcox
mkdir -p /home/bcox/.ssh
chmod 0700 /home/bcox/.ssh
cat > /home/bcox/.ssh/authorized_keys <<-EOF
ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQDPU7h8CaYA1VH/CwY3Ahw0s0wPbB/8t7A96GX/6qS2a8n79nGjywThZJP15L7NnTdTrdV59NEC1QvS0ym/JhlpokXwgMWURsYOAP5Y2lmK7wvAZ65bDn0iPiXXgyWPtEWQqCTV0U9HfZ81m+JMzfcED+L3w0iZAHSeRlupPZRtea3izx91A19RRn0NyVtmrwF4h3g537p+0O3DvaktxZddnwa3vPbY3CE6Eijiqsy9HOrx49YJS3SdBMvGNx91pynVLPWTCziBmYZCt8ioTGNvF8YWLVRf6VCj6M9zTG2NkCbXydAxpfRByTa+4yyKE44hmAehDM15pQGlmcg0O4HlepTqOvPZVyWAvkO3aD1xycrWSTKu68IgRzm9Ve064h3OUqVcWx1tybEAGyioC/H/vdJ4BGKH1wfQQvRbWrO8gCAr8LGS8JUIWWDPOCBtFobsyMo2opck9t8iM8lAiscueMNTJeRuIeK6692m0OsXL9+g8lHJkTD97VF963liCeRhaIG3kIaYXTyOhQdKbDQShT/r4yC8eMWDR/I6ab+2ir/qew46XwHJ98c/Ux0zII5v252D5Q/A4Wf6HJGOjAoMx4iQJ8Q5LYpLnIcX1WqznJQx1zPpaI9WpFe4ELK/mBAv53Emp3HjfacI74/RM6Zt/EHYcKV3Cr5VMaxgLaReaw== bcox@elotl.co
EOF
chmod 0600 /home/bcox/.ssh/authorized_keys
cat /home/bcox/.ssh/authorized_keys

step 'Add itzo group'
addgroup -g 600 -S itzo

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
rc_ulimit='-n 1024000 -p 30112'

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
\${itzo_dir}/itzo-cloud-init --from-metadata-service --from-waagent /var/lib/waagent >> /var/log/itzo/itzo.log 2>&1

itzo_url_file="/tmp/itzo/itzo_url"
itzo_url="http://itzo-download.s3.amazonaws.com"
if [[ -f \$itzo_url_file ]]; then
    itzo_url=\$(head -n 1 \$itzo_url_file)
fi
itzo_version_file="/tmp/itzo/itzo_version"
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
wget -O /usr/local/bin/itzo-cloud-init http://itzo-dev-download.s3.amazonaws.com/itzo-cloud-init-latest
chmod 755 /usr/local/bin/itzo-cloud-init

step 'Add kube-router'
wget -O /usr/local/bin/kube-router http://itzo-dev-download.s3.amazonaws.com/kube-router
chmod 755 /usr/local/bin/kube-router

if [[ "$CLOUD_PROVIDER" == "aws" ]]; then
    # AWS ENA module is included in 4.19 kernels, we just need to enable it

    step 'Add aws-ena module'
    echo ena > /etc/modules-load.d/ena.conf

    # Taken from https://github.com/mcrute/alpine-ec2-ami/blob/master/make_ami.sh
    # Create ENA feature for mkinitfs
    # Submitted upstream: https://github.com/alpinelinux/mkinitfs/pull/19
    echo "kernel/drivers/net/ethernet/amazon/ena" > /etc/mkinitfs/features.d/ena.modules
    # Enable ENA and NVME features these don't hurt for any instance and are
    # hard requirements of the 5 series and i3 series of instances
    sed -Ei 's/^features="([^"]+)"/features="\1 nvme ena"/' /etc/mkinitfs/mkinitfs.conf
    /sbin/mkinitfs $(basename $(find /lib/modules/* -maxdepth 0))
fi

if [[ "$CLOUD_PROVIDER" == "azure" ]]; then
    step "Setup Azure Linux Agent"
    wget -O ./waagent.tar.gz https://github.com/elotl/WALinuxAgent/archive/master.tar.gz && \
	tar xvzf ./waagent.tar.gz && \
	cd WALinuxAgent-master && \
	python setup.py install && \
	cd .. && \
	rm -rf WALinuxAgent-master waagent.tar.gz

    cat > /etc/init.d/waagent <<EOF
#!/sbin/openrc-run
export PATH=/usr/local/sbin:$PATH
start() {
        ebegin "Starting waagent"
        start-stop-daemon --start --exec /usr/sbin/waagent --name waagent -- -start
        eend $? "Failed to start waagent"
}
EOF
    chmod +x /etc/init.d/waagent

    cat > /etc/waagent.conf <<EOF
Provisioning.Enabled=y
Extensions.Enabled=n
Provisioning.UseCloudInit=n
Provisioning.DeleteRootPassword=y
Provisioning.RegenerateSshHostKeyPair=n
Provisioning.SshHostKeyPairType=rsa
Provisioning.MonitorHostName=y
Provisioning.DecodeCustomData=y
Provisioning.ExecuteCustomData=n
Provisioning.AllowResetSysUser=n
ResourceDisk.Format=y
ResourceDisk.Filesystem=ext4
ResourceDisk.MountPoint=/mnt/resource
ResourceDisk.EnableSwap=n
ResourceDisk.SwapSizeMB=0
ResourceDisk.MountOptions=None
Logs.Verbose=n
OS.EnableFIPS=n
OS.RootDeviceScsiTimeout=300
OS.SshClientAliveInterval=30
OS.SshDir=/etc/ssh
OS.EnableRDMA=y
OS.EnableFirewall=n
CGroups.EnforceLimits=n
CGroups.Excluded=customscript,runcommand
AutoUpdate.Enabled=n
EOF

fi

step 'Add NVidia driver'
apk --no-cache add ca-certificates wget
wget -q -O /etc/apk/keys/sgerrand.rsa.pub https://alpine-pkgs.sgerrand.com/sgerrand.rsa.pub
wget https://github.com/sgerrand/alpine-pkg-glibc/releases/download/2.29-r0/glibc-2.29-r0.apk
wget https://github.com/sgerrand/alpine-pkg-glibc/releases/download/2.29-r0/glibc-bin-2.29-r0.apk
wget https://github.com/sgerrand/alpine-pkg-glibc/releases/download/2.29-r0/glibc-dev-2.29-r0.apk
apk add glibc-2.29-r0.apk glibc-bin-2.29-r0.apk glibc-dev-2.29-r0.apk
apk add gcc make musl-dev linux-virt-dev
DRIVER_VERSION=440.64.00
KERNEL_VERSION=$(ls -d /usr/src/linux-headers-* | tail -n1 | sed 's#/usr/src/linux-headers-##g')
wget http://us.download.nvidia.com/tesla/$DRIVER_VERSION/NVIDIA-Linux-x86_64-$DRIVER_VERSION.run
sh NVIDIA-Linux-x86_64-$DRIVER_VERSION.run --ui=none -q -k $KERNEL_VERSION
apk del binutils gmp isl libgomp libatomic mpfr4 mpc1 libstdc++ gcc libbz2 perl libgmpxx gmp-dev elfutils-libelf elfutils-dev ncurses-terminfo-base ncurses-terminfo ncurses-libs readline bash m4 flex bison linux-virt-dev make musl-dev
rm -f NVIDIA-Linux-x86_64-$DRIVER_VERSION.run glibc-*.apk

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

step 'Install nvidia-container-cli'
# This is from https://github.com/NVIDIA/libnvidia-container. To compile in
# extra libraries (not available as compiled against glibc on Alpine)
# statically, build it via:
#
# $ make BIN_LDLIBS="libnvidia-container.a /usr/lib/x86_64-linux-gnu/libseccomp.a /usr/lib/x86_64-linux-gnu/libcap.a -ldl \$(LDLIBS)"
#
wget -O /usr/local/bin/nvidia-container-cli https://s3.amazonaws.com/itzo-packages/nvidia-container-cli
chmod 755 /usr/local/bin/nvidia-container-cli

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

step 'Adding itzo iptables rules'
cat > /etc/init.d/itzo_iptables <<-EOF
#!/sbin/openrc-run

name=\$RC_SVCNAME

depend() {
        need net localmount
}

start() {
  iptables -t nat -A PREROUTING -p tcp --dport 6421 -j ACCEPT
  iptables -t nat -A OUTPUT -p tcp -m owner --gid-owner 600 -j ACCEPT
}
EOF
chmod 755 /etc/init.d/itzo_iptables
cat /etc/init.d/itzo_iptables

step 'Enable vsyscall emulation'
sed -Ei -e "s|^[# ]*(default_kernel_opts)=.*|\1=\"vsyscall=emulate\"|" \
	/etc/update-extlinux.conf
update-extlinux --warn-only 2>&1 | grep -Fv 'extlinux: cannot open device /dev' >&2

step 'Sysctl tweaks'
cat > /etc/sysctl.d/local.conf <<-EOF
fs.file-max = 1024000
EOF

step 'Create ld-linux-x86-64.so.2 link'
mkdir -p /lib64
ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2

step 'Enable services'
rc-update add acpid default
rc-update add chronyd default
rc-update add crond default
rc-update add net.eth0 default
rc-update add sshd default
rc-update add itzo default
rc-update add resizeroot default
rc-update add itzo_iptables default
rc-update add nvidia default
rc-update add net.lo boot
rc-update add termencoding boot
rc-update add haveged boot
if [[ "$CLOUD_PROVIDER" == "azure" ]]; then
    rc-update add waagent default
    rc-update add hv_fcopy_daemon default
    rc-update add hv_kvp_daemon default
    rc-update add hv_vss_daemon default
fi
