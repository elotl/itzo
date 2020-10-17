package util

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

var (
	MaxBufferSize int64 = 1024 * 1024 * 10 // 10MB
)

func TailFile(path string, lines int, maxBytes int64) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	fileSize := info.Size()
	f, err := os.Open(path)
	defer f.Close()

	if err != nil {
		return "", err
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

func CopyFile(src, dst string) error {
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

func IsEmptyDir(name string) (bool, error) {
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

func EnsureFileExists(name string) error {
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
