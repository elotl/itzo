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
from google.oauth2 import service_account
from pyrsistent import PClass, field
from bitmath import GiB, Byte
from threading import Lock
from os import environ
from io import BytesIO
from zope.interface import implementer, Interface

# GCE instances have a metadata server that can be queried for information
# about the instance the code is being run on.
_METADATA_SERVER = u'http://169.254.169.254/computeMetadata/v1/'
_METADATA_HEADERS = {u'Metadata-Flavor': u'Google'}

SCOPES = [u'https://www.googleapis.com/auth/compute']

# Volume defaults based on Flocker data.
VOLUME_DEFAULT_TIMEOUT = 120
VOLUME_LIST_TIMEOUT = 10
VOLUME_DELETE_TIMEOUT = 20
VOLUME_INSERT_TIMEOUT = 20
VOLUME_ATTACH_TIMEOUT = 90
VOLUME_DETATCH_TIMEOUT = 120
SNAPSHOT_DISK_TIMEOUT = 120

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

def make_snapshot(metadata, operations, input, name, snapshot_name):
    print('Connecting')
    size = Byte(image_size(input))
    gce_disk_type = u"pd-standard"
    disk_description = u"Elotl-image-" + name

    print('STEP 1: Creating volume')
    time_point = time.time()
    operations.create_disk(zone=metadata.zone(),
                           name=name,
                           size=size,
                           description=disk_description,
                           gce_disk_type=gce_disk_type)
    print('STEP 1: Took {} seconds to create volume {}'.format(time.time() - time_point, name))
    disk = operations.get_disk_details(name)
    print('STEP 2: Attaching {} to {}'.format(name, metadata.instance_id()))
    result = operations.attach_disk(zone=metadata.zone(),
                                    disk_name=name,
                                    instance_name=metadata.instance_id())

    device_path = u"/dev/disk/by-id/google-" + name
    print('STEP 3: Copying image')
    time_point = time.time()
    copy_image(input, device_path)
    print('STEP 3: Took {} seconds to copy image'.format(time.time() - time_point))
    print('STEP 4; Detaching volume {}'.format(name))
    time_point = time.time()
    result = operations.detach_disk(zone=metadata.zone(),
                                    instance_name=metadata.instance_id(),
                                    disk_name=name)
    print('STEP 4: Took {} seconds to detach volume'.format(time.time() - time_point))
    print('STEP 5: Creating snapshot from {}'.format(name))
    time_point = time.time()
    snapshot_body = {
            'name': snapshot_name
            }
    result = operations.snapshot_disk(project=metadata.project(),
                                      zone=metadata.zone(),
                                      disk_name=name,
                                      body=snapshot_body)
    print('STEP 5: Took {} seconds to create snapshot'.format(time.time() - time_point))
    print('STEP 6: Deleting volume {}'.format(name))
    time_point = time.time()
    # result = operations.destroy_disk(zone=metadata.zone(),
    #                                 disk_name=name)
    print('STEP 6: Took {} seconds to delete volume'.format(time.time() - time_point))
    print('Snapshot {} created\n'.format(snapshot_name))
    return snapshot_name

def make_image_from_snapshot(metadata, operations, name, snapshot_name):
    print('STEP 7: Registering image from {}'.format(snapshot_name))
    time_point = time.time()
    body = {}
    "projects/myechuri-project1/global/snapshots/test-image-snap"
    sourceSnapshot = 'projects/' + metadata.project() + '/global/snapshots/' + snapshot_name
    sourceDisk = 'zones/' + metadata.zone() + '/disks/' + name
    image_body = {
            'name': name,
            # 'sourceSnapshot': sourceSnapshot
            'sourceDisk': sourceDisk
            }
    result = operations.create_image_from_snapshot(project=metadata.project(),
                                                   body=image_body)
    print('STEP 7: Took {} seconds to create image'.format(time.time() - time_point, name))
    print('image {} created\n'.format(name))
    return name

def get_operations(metadata):
    project = metadata.project()
    zone = metadata.zone()
    credentials = service_account.Credentials.from_service_account_file(
                      os.environ['GCE_SERVICE_ACCOUNT_FILE'], scopes=SCOPES)
    compute = discovery.build('compute', 'v1', credentials=credentials)
    operations=GCEOperations(_compute=compute,
                             _project=unicode(project),
                             _zone=unicode(zone))
    return operations
    
class GCEOperations(PClass):
    """
    Class that encompasses all operations that can be done against GCE.

    This separation is done for testing purposes and code cleanliness. Putting
    the operations behind an interface gives us a point of code injection to
    force races that cannot be forced from the higher layer of
    :class:`IBlockDeviceAPI` tests. Also it restricts the use of the GCE
    compute object to this class.

    :ivar _compute: The GCE compute object to use to interact with the GCE API.
    :ivar unicode _project: The project where this block device driver will
        operate.
    :ivar unicode _zone: The zone where this block device driver will operate.
    """
    _compute = field(mandatory=True)
    _project = field(type=unicode, mandatory=True)
    _zone = field(type=unicode, mandatory=True)
    _lock = field(mandatory=True, initial=Lock())

    def _do_blocking_operation(self,
                               function,
                               timeout_sec=VOLUME_DEFAULT_TIMEOUT,
                               sleep=None,
                               **kwargs):
        """
        Perform a GCE operation, blocking until the operation completes.

        This will call `function` with the passed in keyword arguments plus
        additional keyword arguments for project and zone which come from the
        private member variables with the same name. It is expected that
        `function` returns an object that has an `execute()` method that
        returns a GCE operation resource dict.

        This function will then poll the operation until it reaches
        state 'DONE' or times out, and then returns the final
        operation resource dict. The value for the timeout was chosen
        by testing the running time of our GCE operations. Sometimes
        certain operations can take over 30s but they rarely, if ever,
        take over a minute.

        Timeouts should not be caught here but should propogate up the
        stack and the node will eventually retry the operation via the
        convergence loop.

        :param function: Callable that takes keyword arguments project and
            zone, and returns an executable that results in a GCE operation
            resource dict as described above.
        :param int timeout_sec: The maximum amount of time to wait in seconds
            for the operation to complete.
        :param sleep: A callable that has the same signature and function as
            ``time.sleep``. Only intended to be used in tests.
        :param kwargs: Additional keyword arguments to pass to function.

        :returns dict: A dict representing the concluded GCE operation
            resource.
        """
        if sleep is None:
            sleep = time.sleep

        def lock_dropped_sleep(*args, **kwargs):
            """
            A custom sleep function that drops the lock while the actual
            sleeping is going on.
            """
            self._lock.release()
            try:
                return sleep(*args, **kwargs)
            finally:
                self._lock.acquire()

        # args = dict(project=self._project, zone=self._zone)
        args = dict(project=self._project)
        args.update(kwargs)
        with self._lock:
            operation = function(**args).execute()
            return wait_for_operation(
                self._compute, operation, [1]*timeout_sec, lock_dropped_sleep)

    def create_disk(self, zone, name, size, description, gce_disk_type):
        sizeGiB = int(size.to_GiB())
        config = dict(
            name=name,
            sizeGb=sizeGiB,
            description=description,
            type="projects/{project}/zones/{zone}/diskTypes/{type}".format(
                project=self._project, zone=self._zone, type=gce_disk_type)
        )
        return self._do_blocking_operation(
            self._compute.disks().insert,
            zone=zone,
            body=config,
            timeout_sec=VOLUME_INSERT_TIMEOUT,
        )

    def attach_disk(self, zone, disk_name, instance_name):
        config = dict(
            deviceName=disk_name,
            autoDelete=False,
            boot=False,
            source=(
                "https://www.googleapis.com/compute/v1/projects/%s/zones/%s/"
                "disks/%s" % (self._project, self._zone, disk_name)
            )
        )
        return self._do_blocking_operation(
            self._compute.instances().attachDisk,
            zone=zone,
            instance=instance_name,
            body=config,
            timeout_sec=VOLUME_ATTACH_TIMEOUT,
        )

    def detach_disk(self, zone, instance_name, disk_name):
        return self._do_blocking_operation(
            self._compute.instances().detachDisk,
            zone=zone,
            instance=instance_name,
            deviceName=disk_name,
            timeout_sec=VOLUME_DETATCH_TIMEOUT
        )

    def snapshot_disk(self, project, zone, disk_name, body):
        return self._do_blocking_operation(
            self._compute.disks().createSnapshot,
            project=project,
            zone=zone,
            disk=disk_name,
            body=body,
            timeout_sec=SNAPSHOT_DISK_TIMEOUT
        )

    def create_image_from_snapshot(self, project, body):
        return self._do_blocking_operation(
            self._compute.images().insert,
            project=project,
            body=body)

    def destroy_disk(self, zone, disk_name):
        return self._do_blocking_operation(
            self._compute.disks().delete,
            zone=zone,
            disk=disk_name,
            timeout_sec=VOLUME_DELETE_TIMEOUT,
        )

    def list_disks(self, page_token=None, page_size=None):
        with self._lock:
            return self._compute.disks().list(project=self._project,
                                              zone=self._zone,
                                              maxResults=page_size,
                                              pageToken=page_token).execute()

    def get_disk_details(self, disk_name):
        with self._lock:
            return self._compute.disks().get(project=self._project,
                                             zone=self._zone,
                                             disk=disk_name).execute()

    def list_nodes(self, page_token, page_size):
        with self._lock:
            return self._compute.instances().list(
                project=self._project,
                zone=self._zone,
                maxResults=page_size,
                pageToken=page_token
            ).execute()

    def start_node(self, node_id):
        self._do_blocking_operation(
            self._compute.instances().start,
            timeout_sec=5*60,
            instance=node_id
        )

    def stop_node(self, node_id):
        self._do_blocking_operation(
            self._compute.instances().stop,
            timeout_sec=5*60,
            instance=node_id
        )

class LoopExceeded(Exception):
    """
    Raised when ``loop_until`` looped too many times.
    """

    def __init__(self, predicate, last_result):
         super(LoopExceeded, self).__init__(
               '%r never True in loop_until, last result: %r'
               % (predicate, last_result))

def poll_until(predicate, steps, sleep=None):
    if sleep is None:
        sleep = time.sleep
    for step in steps:
        result = predicate()
        if result:
            return result
        sleep(step)
    result = predicate()
    if result:
        return result
    raise LoopExceeded(predicate, result)

def wait_for_operation(compute, operation, timeout_steps, sleep=None):
    """
    Blocks until a GCE operation is complete, or timeout passes.

    This function will then poll the operation until it reaches state
    'DONE' or times out, and then returns the final operation resource
    dict.

    :param compute: The GCE compute python API object.
    :param operation: A dict representing a pending GCE operation resource.
        This can be either a zone or a global operation.
    :param timeout_steps: Iterable of times in seconds to wait until timing out
        the operation.
    :param sleep: a callable taking a number of seconds to sleep while
        polling. Defaults to `time.sleep`

    :returns dict: A dict representing the concluded GCE operation
        resource or `None` if the operation times out.
    """
    poller = _create_poller(operation)

    def finished_operation_result():
        latest_operation = poller.poll(compute)
        if latest_operation['status'] == 'DONE':
            return latest_operation
        return None

    final_operation = poll_until(
        finished_operation_result,
        timeout_steps,
        sleep
    )
    return final_operation

class OperationPoller(Interface):
    """
    Interface for GCE operation resource polling. GCE operation resources
    should be polled from GCE until they reach a status of ``DONE``. The
    specific endpoint that should be polled for the operation is different for
    global operations vs zone operations. This interface provides an
    abstraction for that difference.
    """

    def poll(compute):
        """
        Get the latest version of the requested operation. This should block
        until the latest version of the requested operation is gotten.

        :param compute: The GCE compute python API object.

        :returns: A dict representing the latest version of the GCE operation
            resource.
        """


@implementer(OperationPoller)
class ZoneOperationPoller(PClass):
    """
    Implemenation of :class:`OperationPoller` for zone operations.

    :ivar unicode zone: The zone the operation occurred in.
    :ivar unicode project: The project the operation is under.
    :ivar unicode operation_name: The name of the operation.
    """
    zone = field(type=unicode)
    project = field(type=unicode)
    operation_name = field(type=unicode)

    def poll(self, compute):
        return compute.zoneOperations().get(
            project=self.project,
            zone=self.zone,
            operation=self.operation_name
        ).execute()


@implementer(OperationPoller)
class GlobalOperationPoller(PClass):
    """
    Implemenation of :class:`OperationPoller` for global operations.

    :ivar unicode project: The project the operation is under.
    :ivar unicode operation_name: The name of the operation.
    """
    project = field(type=unicode)
    operation_name = field(type=unicode)

    def poll(self, compute):
        return compute.globalOperations().get(
            project=self.project,
            operation=self.operation_name
        ).execute()


class MalformedOperation(Exception):
    """
    Error indicating that there was an error parsing a dictionary as a GCE
    operation resource.
    """

def _create_poller(operation):
    """
    Creates an operation poller from the passed in operation.

    :param operation: A dict representing a GCE operation resource.

    :returns: An :class:`OperationPoller` provider that can poll the status of
        the operation.
    """
    try:
        operation_name = operation['name']
    except KeyError:
        raise MalformedOperation(
            u"Failed to parse operation, could not find key "
            u"name in: {}".format(operation)
        )
    if 'zone' in operation:
        zone_url_parts = unicode(operation['zone']).split('/')
        try:
            project = zone_url_parts[-3]
            zone = zone_url_parts[-1]
        except IndexError:
            raise MalformedOperation(
                "'zone' key of operation had unexpected form: {}.\n"
                "Expected '(.*/)?<project>/zones/<zone>'.\n"
                "Full operation: {}.".format(operation['zone'], operation))
        return ZoneOperationPoller(
            zone=unicode(zone),
            project=unicode(project),
            operation_name=unicode(operation_name)
        )
    else:
        try:
            project = unicode(operation['selfLink']).split('/')[-4]
        except KeyError:
            raise MalformedOperation(
                u"Failed to parse global operation, could not find key "
                u"selfLink in: {}".format(operation)
            )
        except IndexError:
            raise MalformedOperation(
                "'selfLink' key of operation had unexpected form: {}.\n"
                "Expected '(.*/)?<project>/global/operations/<name>'.\n"
                "Full operation: {}.".format(operation['selfLink'], operation))
        return GlobalOperationPoller(
            project=unicode(project),
            operation_name=unicode(operation_name)
        )

if __name__ == "__main__":
    # Parse arguments
    parser = argparse.ArgumentParser(prog='run')
    parser.add_argument("-n", "--name", action="store", default="test-image",
                        help="image name to be created")
    parser.add_argument("-i", "--input", action="store", default="alpine.qcow2",
                        help="path to the image on local filesystem")

    args = parser.parse_args()
    metadata = Metadata()
    operations = get_operations(metadata)
    snapshot_name = args.name + '-snap'
    snapshot = make_snapshot(metadata, operations, args.input, args.name, snapshot_name)
    make_image_from_snapshot(metadata, operations, args.name, snapshot_name)
