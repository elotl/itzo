package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/golang/glog"
)

const (
	TOSI_MAX_RETRIES = 3
	MaxBufferSize    = 1024 * 1024 * 10 // 10MB
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

func pullAndExtractImage(image, rootfs, url, username, password string) error {
	tp, err := exec.LookPath("tosi")
	if err != nil {
		tp = "/tmp/tosiprg"
		err = downloadTosi(tp)
	}
	if err != nil {
		return err
	}
	args := []string{"-image", image, "-extractto", rootfs}
	if username != "" {
		args = append(args, []string{"-username", username}...)
	}
	if password != "" {
		args = append(args, []string{"-password", password}...)
	}
	if url != "" {
		args = append(args, []string{"-url", url}...)
	}
	return runTosi(tp, args...)
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
