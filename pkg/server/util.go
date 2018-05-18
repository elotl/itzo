package server

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/glog"
)

var (
	TOSI_MAX_RETRIES       = 3
	MaxBufferSize    int64 = 1024 * 1024 * 10 // 10MB
	TOSI_PRG               = "tosi"
)

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}

func resizeVolume() error {
	mounts, err := os.Open("/proc/mounts")
	if err != nil {
		glog.Errorf("opening /proc/mounts: %v", err)
		return err
	}
	defer mounts.Close()
	rootdev := ""
	scanner := bufio.NewScanner(mounts)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, " ")
		if len(parts) < 2 || parts[1] != "/" {
			continue
		} else {
			rootdev = parts[0]
			break
		}
	}
	if err := scanner.Err(); err != nil {
		glog.Errorf("reading /proc/mounts: %v", err)
		return err
	}
	if rootdev == "" {
		err = fmt.Errorf("can't find device the root filesystem is mounted on")
		glog.Error(err)
		return err
	}
	for count := 0; count < 10; count++ {
		// It might take a bit of time for Xen and/or the kernel to detect
		// capacity changes on block devices. The output of resize2fs will
		// contain if it did not need to do anything ("Nothing to do!") vs when
		// it resized the device ("resizing required").
		cmd := exec.Command("resize2fs", rootdev)
		var outbuf bytes.Buffer
		var errbuf bytes.Buffer
		cmd.Stdout = io.MultiWriter(os.Stdout, &outbuf)
		cmd.Stderr = io.MultiWriter(os.Stderr, &errbuf)
		glog.Infof("trying to resize %s", rootdev)
		if err := cmd.Start(); err != nil {
			glog.Errorf("resize2fs %s: %v", rootdev, err)
			return err
		}
		if err := cmd.Wait(); err != nil {
			glog.Errorf("resize2fs %s: %v", rootdev, err)
			return err
		}
		if strings.Contains(outbuf.String(), "resizing required") ||
			strings.Contains(errbuf.String(), "resizing required") {
			glog.Infof("%s has been successfully resized", rootdev)
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	glog.Errorf("resizing %s failed", rootdev)
	return fmt.Errorf("no resizing performed; does %s have new capacity?",
		rootdev)
}

func isEmptyDir(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil && !os.IsExist(err) {
		return true, nil
	} else if err != nil {
		return false, err
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err
}

func ensureFileExists(name string) error {
	f, err := os.Open(name)
	if err != nil && os.IsNotExist(err) {
		f, err = os.Create(name)
	}
	if err != nil {
		return err
	}
	f.Close()
	return nil
}

func downloadTosi(tosipath string) error {
	resp, err := http.Get("http://tosi-download.s3.amazonaws.com/tosi")
	if err != nil {
		return fmt.Errorf("Error creating get request for tosi: %+v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Error downloading tosi: got S3 statuscode %d",
			resp.StatusCode)
	}
	f, err := os.OpenFile(tosipath, os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		return fmt.Errorf("Error opening tosi for writing after download: %+v",
			err)
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return fmt.Errorf("Error writing tosi to filesystem: %+v", err)
	}
	return nil
}

func runTosi(tp string, args ...string) error {
	n := 0
	start := time.Now()
	backoff := 1 * time.Second
	var err error
	for n < TOSI_MAX_RETRIES {
		n++
		var stderr bytes.Buffer
		cmd := exec.Command(tp, args...)
		cmd.Stderr = &stderr
		err = cmd.Run()
		if err == nil {
			glog.Infof("Image download succeeded after %d attempt(s), %v",
				n, time.Now().Sub(start))
			break
		}
		err = fmt.Errorf(
			"Error getting image after %d attempt(s): %+v, output:\n%s",
			n, err, stderr.String())
		glog.Errorf("Image download problem: %v", err)
		glog.Infof("Retrying image download in %v", backoff)
		time.Sleep(backoff)
		backoff = backoff * 2
	}
	return err
}

// I have no idea why I wrote this...  I mean, the host tail
// doesn't quite work right but, for now we're just getting itzo logs
// If nothing else, it was kinda fun.
func tailLines(f *os.File, lines, maxBytes, fileSize int) (string, error) {
	returnParts := make([][]byte, 0)
	chunkSize := 8196
	curChunk := 0
	linesSeen := 0

	for linesSeen < lines &&
		curChunk*chunkSize < fileSize &&
		curChunk*chunkSize < maxBytes {

		chunkBuf := make([]byte, chunkSize)
		curChunk += 1
		offsetFromEnd := curChunk * chunkSize
		offsetFromStart := fileSize - offsetFromEnd
		if offsetFromStart < 0 {
			chunkBuf = make([]byte, chunkSize+offsetFromStart)
			offsetFromStart = 0
		}
		_, _ = f.ReadAt(chunkBuf, int64(offsetFromStart))

		linesSeen += bytes.Count(chunkBuf, []byte("\n"))
		if linesSeen > lines {
			overCount := linesSeen - lines

			parts := bytes.Split(chunkBuf, []byte("\n"))
			if overCount < len(parts) {
				parts = parts[overCount:]
				returnParts = append(returnParts, bytes.Join(parts, []byte("\n")))
			}
		} else {
			returnParts = append(returnParts, chunkBuf)
		}
	}
	// We could do this with a single buffer but... nah{
	var returnBuffer bytes.Buffer
	for i := len(returnParts) - 1; i >= 0; i-- {
		returnBuffer.Write(returnParts[i])
	}
	return returnBuffer.String(), nil
}

func tailBytes(f *os.File, maxBytes, fileSize int64) (string, error) {
	if maxBytes > fileSize {
		maxBytes = fileSize
	}
	buf := make([]byte, maxBytes)
	if fileSize > maxBytes {
		f.Seek(-maxBytes, 2)
	}
	_, err := f.Read(buf)
	if err != nil {
		return "", fmt.Errorf("Error reading file: %s", err)
	}
	return string(buf), nil
}

func tailFile(path string, lines int, maxBytes int64) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("Could not stat file: %s", err)
	}
	fileSize := info.Size()
	f, err := os.Open(path)
	defer f.Close()

	if err != nil {
		return "", fmt.Errorf("Error opening file: %s", err)
	}
	if maxBytes == 0 || maxBytes > MaxBufferSize {
		maxBytes = MaxBufferSize
	}

	if lines > 0 {
		return tailLines(f, lines, int(maxBytes), int(fileSize))
	} else {
		return tailBytes(f, maxBytes, fileSize)
	}
}

func minint64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
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
