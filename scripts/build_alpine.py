import boto3
import datetime
import urllib
import time
import os
import subprocess


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


def safe_mkdir(dirpath):
    if not os.path.exists(dirpath):
        os.makedirs(dirpath)


def get_script_dir():
    return os.path.dirname(os.path.realpath(__file__))


def setup():
    print('Installing dependencies')
    cmd = 'sudo apt-get install qemu expect'
    subprocess.check_call(cmd, shell=True)
    isodir = os.path.join(get_script_dir(), 'iso')
    safe_mkdir(isodir)
    print('Downloading ISO')
    isofile = os.path.join(isodir, 'alpine-virt-3.6.2-x86_64.iso')
    if not os.path.exists(isofile):
        cmd = 'wget --directory-prefix=./iso http://dl-cdn.alpinelinux.org/alpine/v3.6/releases/x86_64/alpine-virt-3.6.2-x86_64.iso'
        subprocess.check_call(cmd, shell=True)


def create_volume():
    metadata = Metadata()
    print('Connecting to ec2')
    ec2 = boto3.resource('ec2', region_name=metadata.region())
    print('STEP 1: Creating volume')  # aws ec2 create-volume
    time_point = time.time()
    ts = datetime.datetime.utcnow().strftime("%Y%m%d-%H:%M:%S")
    vol = ec2.create_volume(
        Size=1,
        AvailabilityZone=metadata.availability_zone(),
        VolumeType='standard',
        TagSpecifications=[
            {
                'ResourceType': 'volume',
                'Tags': [
                    {
                        'Key': 'Name',
                        'Value': 'MilpaAlpine-{}'.format(ts)
                    },
                ]
            },
        ]
    )
    print('Waiting for {}'.format(vol.id))
    wait_for(vol)
    print('STEP 1: Took {} seconds to create volume {}'.format(
        time.time() - time_point, vol.id))
    print('STEP 2: Attaching {} to {}'.format(
        vol.id, metadata.instance_id()))  # aws ec2 attach-volume
    time_point = time.time()
    vol.attach_to_instance(InstanceId=metadata.instance_id(), Device='xvdf')
    while not os.path.exists('/dev/xvdf'):
        print('waiting for volume to attach')
        time.sleep(1)
    return vol


def install():
    print('STEP 3: Creating Alpine from ISO')
    subprocess.check_call("sudo ./expect_qemu.tcl", shell=True)
    # at this point /dev/xvdf has been partitioned and has alpine linux on it


def provision():
    subprocess.check_call("sudo bash ./provision.sh", shell=True)


def make_snapshot(vol):
    time_point = time.time()
    vol.detach_from_instance()
    print('STEP 4: Took {} seconds to detach volume'.format(
        time.time() - time_point))
    print('STEP 5: Creating snapshot from {}'.format(vol.id))
    time_point = time.time()
    snap = vol.create_snapshot()
    wait_for(snap, 'completed')
    print('STEP 5: Took {} seconds to create snapshot {}'.format(
        time.time() - time_point, snap.id))
    print('STEP 6: Deleting volume {}'.format(vol.id))
    time_point = time.time()
    vol.delete()
    print('STEP 6: Took {} seconds to delete volume'.format(
        time.time() - time_point))
    print('Snapshot {} created\n'.format(snap))
    return snap.id


def make_ami_from_snapshot(name, snapshot_id):
    metadata = Metadata()
    print('Connecting')
    ec2 = boto3.resource('ec2', region_name=metadata.region())
    print('STEP 7: Registering image from {}'.format(snapshot_id))
    time_point = time.time()
    ami = ec2.register_image(Name=name,
                             Architecture='x86_64',
                             RootDeviceName='xvda',
                             VirtualizationType='hvm',
                             BlockDeviceMappings=[
                                 {
                                     'DeviceName': 'xvda',
                                     'Ebs': {
                                         'SnapshotId': snapshot_id,
                                         'DeleteOnTermination': True
                                     }
                                 },
                             ])
    print('STEP 7: Took {} seconds to create ami'.format(
        time.time() - time_point, ami))
    print('ami {} created\n'.format(ami))
    return ami


if __name__ == '__main__':
    setup()
    vol = create_volume()
    install()
    provision()
    snapshot_id = make_snapshot(vol)
    ts = datetime.datetime.utcnow().strftime("%Y%m%d-%H%M00")
    ami_name = 'milpa-alpine-{}'.format(ts)
    make_ami_from_snapshot(ami_name, snapshot_id)
