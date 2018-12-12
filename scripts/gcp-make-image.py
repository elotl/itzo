#!/usr/bin/python2

# The following documentation is useful while maintaining this file:
# Python client: https://cloud.google.com/compute/docs/tutorials/python-guide
# Rest API: https://cloud.google.com/compute/docs/reference/rest/v1/
# Python Oauth: https://developers.google.com/identity/protocols/OAuth2ServiceAccount#authorizingrequests

import argparse, urllib, time, json, subprocess, os.path
import requests
from googleapiclient import discovery
from googleapiclient.errors import HttpError
from oauth2client.client import GoogleCredentials
from oauth2client.service_account import ServiceAccountCredentials

# GCE instances have a metadata server that can be queried for information
# about the instance the code is being run on.
_METADATA_SERVER = u'http://169.254.169.254/computeMetadata/v1/'
_METADATA_HEADERS = {u'Metadata-Flavor': u'Google'}

class Metadata(object):
    def _get_metadata_path(self, path):
        """
        Requests a metadata path from the metadata server available within GCE.

        :param unicode path: The path on the metadata server to query.
        :returns unicode: The resulting value from the metadata server.
        """
        timeout_sec = 3
        r = requests.get(_METADATA_SERVER + path,
                         headers=_METADATA_HEADERS,
                         timeout=timeout_sec)
        if r.status_code != 200:
            raise ValueError("Did not get success result from metadata server "
                             "for path {}, instead got {}.".format(
                             path, r.status_code))
        return r.text
    def instance_id(self):
        """
        GCE does operations based on the `name` of resources, and also
        assigns the name to the hostname. Users can change the
        system's hostname but the metadata server's hostname attribute
        will return the original instance name. Thus, we use that as the
        source of the hostname.
        """
        fqdn = self._get_metadata_path("instance/hostname")
        return unicode(fqdn.split(".")[0])
    def zone(self):
        return self._get_metadata_path('instance/zone').split('/')[-1]
    def project(self):
        return self._get_metadata_path('project/project-id')

def wait_for(ec2_obj, status='available'):
    while ec2_obj.state != status:
        print('object status {} wanted {}'.format(ec2_obj.state, status))
        time.sleep(1)
        ec2_obj.reload()

def to_bytes(size):
    gib = 1 << 30
    return (size + gib - 1) >> 30

def image_size(filename):
    info = json.loads(subprocess.check_output(['qemu-img', 'info', '--output=json', filename]))
    return info['virtual-size']

def copy_image(img, out):
    subprocess.check_call(['sudo', 'qemu-img', 'convert', '-O', 'raw', img, out])

def make_snapshot(metadata, operations, input, name):
    print('Connecting')
    size = Byte(image_size(input))
    gce_disk_type = ValueConstant(GCEDiskTypes.STANDARD.value)

    print('STEP 1: Creating volume')
    time_point = time.time()
    try:
       operations.create_disk(size=size,
                              gce_disk_type=gce_disk_type)

    print('Waiting for {}'.format(vol.id))
    wait_for(vol)
    print('STEP 1: Took {} seconds to create volume {}'.format(time.time() - time_point,vol.id))
    print('STEP 2: Attaching {} to {}'.format(vol.id, metadata.instance_id())) # aws ec2 attach-volume
    time_point = time.time()
    vol.attach_to_instance(InstanceId=metadata.instance_id(), Device='xvdf')
    while not os.path.exists('/dev/xvdf'):
        print('waiting for volume to attach')
        time.sleep(1)
    print('STEP 2: Took {} seconds to attach volume to instance {}'.format(time.time() - time_point,metadata.instance_id()))
    print('STEP 3: Copying image')
    time_point = time.time()
    copy_image(input, '/dev/xvdf')
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

def get_operations(metadata):
    project = metadata.project()
    zone = metadata.zone()
    gce_credentials = GoogleCredentials.get_application_default()
    compute = discovery.build('compute', 'v1', credentials=gce_credentials)
    operations=GCEOperations(_compute=compute,
                             _project=unicode(project),
                             _zone=unicode(zone))
    return operations
    
if __name__ == "__main__":
    # Parse arguments
    parser = argparse.ArgumentParser(prog='run')
    parser.add_argument("-n", "--name", action="store", default="test-image",
                        help="image name to be created")
    parser.add_argument("-i", "--input", action="store", default="build/release.x64/usr.img",
                        help="path to the image on local filesystem")

    args = parser.parse_args()
    metadata = Metadata()
    operations = get_operations(metadata)
    snapshot = make_snapshot(metadata, operations, args.input, args.name)
    make_ami_from_snapshot(args.name,snapshot.id)
    #snapshot.delete()
