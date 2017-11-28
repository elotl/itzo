#!/usr/bin/expect -f

set timeout -1

#Assign a variable to the log file
# set log     [lindex $argv 0]

#Start the guest VM
spawn qemu-system-x86_64 -cdrom ./iso/alpine-virt-3.6.2-x86_64.iso -drive format=raw,file=/dev/xvdf -boot d -net nic -net user -m 256 -localtime -nographic

#Login process
expect "localhost login: "
#Enter username
send "root\n"

expect "localhost:~# "
send "export empty_root_password=1\n"
expect "localhost:~# "
send "echo KEYMAPOPTS='\"us us\"' > baz\n"
expect "localhost:~# "
send "echo HOSTNAMEOPTS='\"-n alpinist\"' >> baz\n"
expect "localhost:~# "
send "echo INTERFACESOPTS='\"auto lo' >> baz\n"
expect "localhost:~# "
send "echo 'iface lo inet loopback' >> baz\n"
expect "localhost:~# "
send "echo '' >> baz\n"
expect "localhost:~# "
send "echo 'auto eth0' >> baz\n"
expect "localhost:~# "
send "echo 'iface eth0 inet dhcp' >> baz\n"
expect "localhost:~# "
send "echo '    hostname alpinist' >> baz\n"
expect "localhost:~# "
send "echo '\"' >> baz\n"
expect "localhost:~# "
send "echo PROXYOPTS='\"none\"' >> baz\n"
expect "localhost:~# "
send "echo TIMEZONEOPTS='\"-z UTC\"' >> baz\n"
expect "localhost:~# "
send "echo APKREPOSOPTS='\"-f\"' >> baz\n"
expect "localhost:~# "
send "echo SSHDOPTS='\"-c openssh\"' >> baz\n"
expect "localhost:~# "
send "echo NTPOPTS='\"-c openntpd\"' >> baz\n"
expect "localhost:~# "
send "echo DISKOPTS='\"-m sys /dev/sda\"' >> baz\n"
expect "localhost:~# "
send "echo LBUOPTS='\"none\"' >> baz\n"
expect "localhost:~# "
send "echo APKCACHEOPTS='\"/var/cache/apk\"' >> baz\n"

expect "localhost:~# "
send "setup-alpine -f baz\n"

expect "WARNING: Erase the above disk(s) and continue? \\\[y/N\\\]: "
send "y\n"

# while 1 {
#     expect {
# 	"WARNING: Erase the above disk(s) and continue? \\\[y/N\\\]: "    { send "y\n"; break }
# 	default  { continue }
#     }
# }
#Enter where to store configs ('floppy', 'usb' or 'none') [none]:
#Enter apk cache directory (or '?' or 'none') [/var/cache/apk]:


#poweroff the Guest VM
expect "alpinist:~# "
send "poweroff\r"
