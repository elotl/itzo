package util

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
	"golang.org/x/mod/semver"
)

const (
	semverRegexFmt string = `v?([0-9]+)(\.[0-9]+)(\.[0-9]+)?` +
		`(-([0-9A-Za-z\-]+(\.[0-9A-Za-z\-]+)*))*` +
		`(\+([0-9A-Za-z\-]+(\.[0-9A-Za-z\-]+)*))?`
)

var (
	semverRegex = regexp.MustCompile("^" + semverRegexFmt + "$")
	itzoBinPath = "/tmp/itzo/bin"
)

func versionMatch(exe, minVersion, versionArg string) bool {
	cmd := exec.Command(exe, versionArg)
	output, err := cmd.CombinedOutput()
	if err != nil {
		glog.V(2).Infof("%q error getting version: %v", exe, err)
		return false
	}
	version := ""
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		for _, word := range strings.Fields(line) {
			word = strings.TrimRight(word, ",;.")
			if semverRegex.Match([]byte(word)) {
				version = word
				glog.V(2).Infof("%q found version %q (minimum requested: %q)",
					exe, version, minVersion)
				return semver.Compare(version, minVersion) >= 0
			}
		}
	}
	glog.V(2).Infof("%q not found version in output %q", exe, output)
	return false
}

func ensurePath(localPath string) {
	envPath := os.Getenv("PATH")
	for _, p := range strings.Split(envPath, ":") {
		if p == localPath {
			return
		}
	}
	os.Setenv("PATH", envPath+":"+localPath)
}

func EnsureProg(prog, downloadURL, minVersion, versionArg string) (string, error) {
	ensurePath(itzoBinPath)
	exe, err := exec.LookPath(prog)
	if err == nil {
		found := versionMatch(exe, minVersion, versionArg)
		glog.V(5).Infof("looking for %s %s: found %v", exe, minVersion, found)
		if found {
			return exe, nil
		}
	}
	// Install into our bin directory in case there is an older version present
	// in /usr/bin or /usr/local/bin.
	progBase := filepath.Base(prog)
	exe = filepath.Join(itzoBinPath, progBase)
	err = InstallProg(downloadURL, exe)
	if err != nil {
		return "", err
	}
	return exe, nil
}

func InstallProg(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("creating get request for %s: %+v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("downloading %s: got status code %d",
			url, resp.StatusCode)
	}
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		return fmt.Errorf("opening %s for writing: %+v", path, err)
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return fmt.Errorf("writing %s: %+v", path, err)
	}
	return nil
}

func RunProg(prog string, outputLimit, maxRetries int, args ...string) error {
	glog.Infof("running %s with args %v", prog, args)
	n := 0
	start := time.Now()
	backoff := 3
	for {
		n++
		var stderr bytes.Buffer
		cmd := exec.Command(prog, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err == nil {
			glog.Infof("%s succeeded after %d attempt(s), %v",
				prog, n, time.Now().Sub(start))
			return err
		}
		freebs, availbs, serr := GetNumberOfFreeAndAvailableBlocks("/")
		if serr == nil && (freebs < 5 || availbs < 5) {
			err = fmt.Errorf(
				"low disk space, free/available blocks: %d/%d", freebs, availbs)
			glog.Errorf("running %s: %v", prog, err)
			return err
		}
		output := stderr.String()
		if len(output) > outputLimit {
			output = output[len(output)-outputLimit:]
		}
		err = fmt.Errorf("%s failed after %d attempt(s): %+v, output:\n%s",
			prog, n, err, output)
		glog.Errorf("running %s %v: %v", prog, args, err)
		if n >= maxRetries {
			return err
		}
		glog.Infof("retrying %s %v", prog, args)
		time.Sleep(time.Duration(backoff) * time.Second)
		jitter := int(math.Ceil(rand.Float64() * float64(backoff)))
		backoff = backoff*2 + jitter
	}
}
