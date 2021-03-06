/*
Copyright 2020 Elotl Inc

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
)

// simply grabs the partition number from the end of the device name
func getPartitionNumber(dev string) (string, error) {
	if len(dev) == 0 {
		return "", fmt.Errorf("could not get partition number from empty string")
	}
	return string(dev[len(dev)-1]), nil
}

// The user can tell kip to enlarge the root volume when dispatching
// (on AWS and GCE). We can enlarge the root partition to fill up all
// the available space on the disk. Summary of the algorithm:
//
// 1. From /proc/mounts, get the partition that is mounted
//    as / (e.g. /dev/nvme0n1p1).
// 2. Get the disk device of the partition from 1. (e.g. nvme0n1).
// 3. If we found the disk device, extend the partition via updating
//    the partition table using "growpart".
// 4. Resize the filesystem on the partition. For now, we only support
//    ext[34] via resize2fs.
//
// This will fail on systems that don't have the necessary executables
// and cases when the root partition cannot be enlarged (when there is
// another partition after the root partition).
func resizeVolume() error {
	mounts, err := os.Open("/proc/mounts")
	if err != nil {
		err = errors.Wrap(err, "opening /proc/mounts")
		glog.Error(err)
		return err
	}
	defer mounts.Close()
	// rootPartition will look something like /dev/nvme0n1p1
	rootPartition := ""
	scanner := bufio.NewScanner(mounts)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, " ")
		if len(parts) < 2 || parts[1] != "/" {
			continue
		} else {
			rootPartition = parts[0]
			break
		}
	}
	if err := scanner.Err(); err != nil {
		err = errors.Wrap(err, "reading /proc/mounts")
		glog.Error(err.Error())
		return err
	}
	if rootPartition == "" {
		err = fmt.Errorf("can't find device the root filesystem partition is mounted on")
		glog.Error(err)
		return err
	}
	// Grab the root partition's raw device (e.g. /dev/nvme0n1)
	out, err := exec.Command("lsblk", "-no", "pkname", rootPartition).Output()
	if err != nil {
		err = errors.Wrap(err, "Could not get the root partition's root block device:")
		glog.Error(err)
		return err
	}
	rootDevice := "/dev/" + strings.TrimSpace(string(out))

	// if we have a partitioned device then we need to grow the root partition
	// otherwise the disk has no partitions and we can skip this step
	if rootDevice != rootPartition {
		partitionNumber, err := getPartitionNumber(rootPartition)
		if err != nil {
			return err
		}

		var result error
		for count := 0; count < 30; count++ {
			out, err = exec.Command("growpart", rootDevice, partitionNumber).CombinedOutput()
			if err == nil {
				result = nil
				break
			}
			if strings.Contains(string(out), "NOCHANGE") &&
				strings.Contains(string(out), "cannot be grown") {
				err = fmt.Errorf("partition cannot be grown: %v; output: %q", err, out)
				result = multierror.Append(result, err)
				glog.Warningf("%v", err)
				time.Sleep(1)
				continue
			}
			err = fmt.Errorf("could not grow root partition: %v; output: %q", err, out)
			glog.Error(err)
			return err
		}
		if result != nil {
			return result
		}
	}

	var result error
	for count := 0; count < 15; count++ {
		// It might take a bit of time for Xen and/or the kernel to detect
		// capacity changes on block devices. The output of resize2fs will
		// contain if it did not need to do anything ("The filesystem is
		// already 4980475 (4k) blocks long. Nothing to do!") vs when it
		// resized the device ("resizing required").
		cmd := exec.Command("resize2fs", rootPartition)
		var outbuf bytes.Buffer
		var errbuf bytes.Buffer
		cmd.Stdout = io.MultiWriter(os.Stdout, &outbuf)
		cmd.Stderr = io.MultiWriter(os.Stderr, &errbuf)
		glog.Infof("trying to resize %s", rootPartition)
		if err := cmd.Start(); err != nil {
			err = errors.Wrapf(err, "resize2fs %s", rootPartition)
			glog.Error(err)
			return err
		}
		if err := cmd.Wait(); err != nil {
			err = errors.Wrapf(err, "resize2fs %s: stdout: %q stderr: %q", rootPartition, outbuf.String(), errbuf.String())
			glog.Error(err)
			return err
		}
		if strings.Contains(outbuf.String(), "resizing required") ||
			strings.Contains(errbuf.String(), "resizing required") {
			glog.Infof("%s has been successfully resized", rootPartition)
			return nil
		}
		err := fmt.Errorf("resize2fs %s: stdout: %q stderr: %q", rootPartition, outbuf.String(), errbuf.String())
		result = multierror.Append(result, err)
		time.Sleep(1 * time.Second)
	}
	glog.Errorf("resizing %s failed: %v", rootPartition, result)
	return fmt.Errorf("no resizing performed; does %s have new capacity? %v", rootPartition, result)
}

type Link struct {
	dst      string
	src      string
	linktype byte
	mode     os.FileMode
	uid      int
	gid      int
}

func DeployPackage(filename, rootdir, pkgname string) (err error) {
	destdir := filepath.Join(rootdir, "..", "packages", pkgname)
	glog.Infof("Deploying package %s to %s", filename, destdir)

	err = os.MkdirAll(destdir, 0700)
	if err != nil {
		glog.Errorf("Creating directory %s for package %s: %v",
			destdir, filename, err)
		return err
	}

	err = doDeployPackage(filename, destdir)
	if err != nil {
		return err
	}

	return nil
}

func doDeployPackage(filename, destdir string) (err error) {
	f, err := os.Open(filename)
	if err != nil {
		glog.Errorln("opening package file:", err)
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		glog.Errorln("uncompressing package:", err)
		return err
	}
	defer gzr.Close()

	var links []Link

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			glog.Errorln("extracting package:", err)
			return err
		}

		name := header.Name
		if name == "ROOTFS" {
			continue
		}
		if len(name) < 7 || name[:7] != "ROOTFS/" {
			glog.Warningln("file outside of ROOTFS in package:", name)
			continue
		}
		name = filepath.Join(destdir, name[7:])

		dirname := filepath.Dir(name)
		if _, err = os.Stat(dirname); os.IsNotExist(err) {
			os.MkdirAll(dirname, 0755)
		}

		switch header.Typeflag {
		case tar.TypeDir: // directory
			glog.Infoln("d", name)
			os.Mkdir(name, os.FileMode(header.Mode))
		case tar.TypeReg: // regular file
			glog.Infoln("f", name)
			data := make([]byte, header.Size)
			read_so_far := int64(0)
			for read_so_far < header.Size {
				n, err := tr.Read(data[read_so_far:])
				if err != nil && err != io.EOF {
					glog.Errorln("extracting", name, ":", err)
					return err
				}
				read_so_far += int64(n)
			}
			if read_so_far != header.Size {
				glog.Errorf("f %s error: read %d bytes, but size is %d bytes", name, read_so_far, header.Size)
			}
			ioutil.WriteFile(name, data, os.FileMode(header.Mode))
		case tar.TypeLink, tar.TypeSymlink:
			linkname := header.Linkname
			if len(linkname) >= 7 && linkname[:7] == "ROOTFS/" {
				linkname = filepath.Join(destdir, linkname[7:])
			}
			// Links might point to files or directories that have not been
			// extracted from the tarball yet. Create them after going through
			// all entries in the tarball.
			links = append(links, Link{linkname, name, header.Typeflag, os.FileMode(header.Mode), header.Uid, header.Gid})
			continue
		default:
			glog.Warningf("unknown type while untaring: %d", header.Typeflag)
			continue
		}
		err = os.Chown(name, header.Uid, header.Gid)
		if err != nil {
			glog.Warningf("warning: chown %s type %d uid %d gid %d: %v", name, header.Typeflag, header.Uid, header.Gid, err)
		}
	}

	for _, link := range links {
		os.Remove(link.src) // Remove link in case it exists.
		if link.linktype == tar.TypeSymlink {
			glog.Infoln("s", link.src)
			err = os.Symlink(link.dst, link.src)
			if err != nil {
				glog.Errorf("creating symlink %s -> %s: %v", link.src, link.dst, err)
				return err
			}
			err = os.Lchown(link.src, link.uid, link.gid)
			if err != nil {
				glog.Warningf("warning: chown symlink %s uid %d gid %d: %v", link.src, link.uid, link.gid, err)
			}
		}
		if link.linktype == tar.TypeLink {
			glog.Infoln("h", link.src)
			err = os.Link(link.dst, link.src)
			if err != nil {
				glog.Errorf("creating hardlink %s -> %s: %v", link.src, link.dst, err)
				return err
			}
			err = os.Chmod(link.src, link.mode)
			if err != nil {
				glog.Errorf("chmod hardlink %s %d: %v", link.src, link.mode, err)
				return err
			}
			err = os.Chown(link.src, link.uid, link.gid)
			if err != nil {
				glog.Warningf("warning: chown hardlink %s uid %d gid %d: %v", link.src, link.uid, link.gid, err)
			}
		}
	}

	return nil
}

// This will ensure all the helper processes and their children get terminated
// before the main process exits.
func KillChildren() {
	// Set of pids.
	var pids map[int]interface{} = make(map[int]interface{})
	pids[os.Getpid()] = nil

	d, err := os.Open("/proc")
	if err != nil {
		return
	}
	defer d.Close()

	for {
		fis, err := d.Readdir(10)
		if err == io.EOF {
			break
		}
		if err != nil {
			return
		}

		for _, fi := range fis {
			if !fi.IsDir() {
				continue
			}
			name := fi.Name()
			if name[0] < '0' || name[0] > '9' {
				continue
			}
			pid64, err := strconv.ParseInt(name, 10, 0)
			if err != nil {
				continue
			}
			pid := int(pid64)
			statPath := fmt.Sprintf("/proc/%s/stat", name)
			dataBytes, err := ioutil.ReadFile(statPath)
			if err != nil {
				continue
			}
			data := string(dataBytes)
			binStart := strings.IndexRune(data, '(') + 1
			binEnd := strings.IndexRune(data[binStart:], ')')
			data = data[binStart+binEnd+2:]
			var state int
			var ppid int
			var pgrp int
			var sid int
			_, _ = fmt.Sscanf(data, "%c %d %d %d", &state, &ppid, &pgrp, &sid)
			_, ok := pids[ppid]
			if ok {
				syscall.Kill(pid, syscall.SIGKILL)
				// Kill any children of this process too.
				pids[pid] = nil
			}
		}
	}
}

func NewTestServer(store EnvStore, rootdir string, podCtl *PodController) Server {
	s := Server{
		env:            store,
		installRootdir: rootdir,
		podController:  podCtl,
	}
	s.getHandlers()
	s.primaryIP = "fake-ip"
	return s
}
