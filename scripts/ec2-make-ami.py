#!/usr/bin/python2

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

import boto3, argparse, urllib, time, json, subprocess, os.path
import argparse

class Metadata(object):
    base = 'http://169.254.169.254/latest/meta-data/'
    def _get(self, what):
        return urllib.urlopen(Metadata.base + what).read()
    def instance_id(self):
        return self._get('instance-id')
    def availability_zone(self):
        return self._get('placement/availability-zone')
    def region(self):
        return self.availability_zone()[:-1]

def wait_for(ec2_obj, status='available'):
    while ec2_obj.state != status:
        print('object status {} wanted {}'.format(ec2_obj.state, status))
        time.sleep(1)
        ec2_obj.reload()

def to_gib(size):
    gib = 1 << 30
    return (size + gib - 1) >> 30

def image_size(filename):
    info = json.loads(subprocess.check_output(['qemu-img', 'info', '--output=json', filename]))
    return info['virtual-size']

def copy_image(img, out):
    subprocess.check_call(['sudo', 'qemu-img', 'convert', '-O', 'raw', img, out])

def wait_for_device(dev):
    lsblk_output = subprocess.check_output(['lsblk'])
    for line in lsblk_output.split('\n'):
        if line.startswith('nvme'):
            dev = '/dev/nvme1n1'
    while not os.path.exists(dev):
        print('waiting for volume to attach')
        time.sleep(1)
    return dev

def make_snapshot(input, name):
    metadata = Metadata()
    print('Connecting')
    ec2 = boto3.resource('ec2',region_name=metadata.region())
    print('STEP 1: Creating volume') # aws ec2 create-volume
    time_point = time.time()
    vol = ec2.create_volume(Size=to_gib(image_size(input)),
                            AvailabilityZone=metadata.availability_zone(),
                            VolumeType='gp2',
                            )
    print('Waiting for {}'.format(vol.id))
    wait_for(vol)
    print('STEP 1: Took {} seconds to create volume {}'.format(time.time() - time_point,vol.id))
    print('STEP 2: Attaching {} to {}'.format(vol.id, metadata.instance_id())) # aws ec2 attach-volume
    time_point = time.time()
    vol.attach_to_instance(InstanceId=metadata.instance_id(), Device='xvdf')
    dev = wait_for_device('/dev/xvdf')
    print('STEP 2: Took {} seconds to attach volume to instance {} device {}'.format(time.time() - time_point,metadata.instance_id(),dev))
    print('STEP 3: Copying image')
    time_point = time.time()
    copy_image(input, dev)
    print('STEP 3: Took {} seconds to copy image'.format(time.time() - time_point))
    print('STEP 4; Detaching volume {}'.format(vol.id))
    time_point = time.time()
    vol.detach_from_instance()
    print('STEP 4: Took {} seconds to detach volume'.format(time.time() - time_point))
    print('STEP 5: Creating snapshot from {}'.format(vol.id)) # aws ec2 create-snapshot
    time_point = time.time()
    snap = vol.create_snapshot(Description='snap-%s' % name)
    wait_for(snap, 'completed')
    print('STEP 5: Took {} seconds to create snapshot {}'.format(time.time() - time_point,snap.id))
    print('STEP 6: Deleting volume {}'.format(vol.id))
    time_point = time.time()
    vol.delete()
    print('STEP 6: Took {} seconds to delete volume'.format(time.time() - time_point))
    print('Snapshot {} created\n'.format(snap))
    return snap

def make_ami_from_snapshot(name,snapshot_id):
    metadata = Metadata()
    print('Connecting')
    ec2 = boto3.resource('ec2',region_name=metadata.region())
    print('STEP 7: Registering image from {}'.format(snapshot_id)) # aws ec2 register-image
    time_point = time.time()
    ami = ec2.register_image(Name=name,
                             Architecture='x86_64',
                             RootDeviceName='xvda',
                             VirtualizationType='hvm',
                             EnaSupport=True,
                             BlockDeviceMappings=[
                                 {
                                     'DeviceName' : 'xvda',
                                     'Ebs': {
                                         'SnapshotId': snapshot_id,
                                         'DeleteOnTermination': True
                                     }
                                 },
                             ])
    print('STEP 7: Took {} seconds to create ami'.format(time.time() - time_point,ami))
    print('ami {} created\n'.format(ami))
    return ami

if __name__ == "__main__":
    # Parse arguments
    parser = argparse.ArgumentParser(prog='run')
    parser.add_argument("-n", "--name", action="store", default="test-ami",
                        help="ami name to be created")
    parser.add_argument("-i", "--input", action="store", default="build/release.x64/usr.img",
                        help="path to the image on local filesysten")

    args = parser.parse_args()
    snapshot = make_snapshot(args.input, args.name)
    make_ami_from_snapshot(args.name,snapshot.id)
    #snapshot.delete()
