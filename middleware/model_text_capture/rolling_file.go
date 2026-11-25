// sudoapi: Model request and response text capture.

package model_text_capture

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const backupTimeLayout = "2006-0102-150405"

type rollingFile struct {
	handle        rollingFileHandle
	size          int64
	prunedBackups int
	incomplete    map[string]struct{}
}

type rollingFileHandle interface {
	io.WriteCloser
	Name() string
}

func (file *rollingFile) write(data []byte, name string, maxSizeMiB int64, maxBackups int) error {
	filename := filepath.Clean(name)
	if file.handle != nil && file.handle.Name() != filename {
		if err := file.close(); err != nil {
			return err
		}
	}
	if strings.TrimSpace(name) == "" || maxSizeMiB <= 0 || maxSizeMiB > math.MaxInt64>>20 || maxBackups < 0 {
		return errors.New("invalid rolling file settings")
	}

	maxBytes := maxSizeMiB << 20
	dataSize := int64(len(data))
	if dataSize > maxBytes {
		return errors.New("write exceeds the file size limit")
	}

	if file.handle == nil {
		if err := file.open(filename); err != nil {
			return err
		}
	}
	if file.size > maxBytes-dataSize {
		if err := file.close(); err != nil {
			return err
		}
		if err := rotateFile(filename); err != nil {
			return err
		}
		if err := file.open(filename); err != nil {
			return err
		}
	}

	written, err := file.handle.Write(data)
	if err == nil && written < len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		if written > 0 {
			return errors.Join(err, file.rollbackPartialWrite(filename))
		}
		return err
	}
	file.size += int64(written)
	if maxBackups != file.prunedBackups {
		if err := pruneFileBackups(filename, maxBackups); err != nil {
			return err
		}
		file.prunedBackups = maxBackups
	}
	return nil
}

func (file *rollingFile) rollbackPartialWrite(filename string) error {
	size := file.size
	closeErr := file.close()
	if err := os.Truncate(filename, size); err != nil {
		if file.incomplete == nil {
			file.incomplete = make(map[string]struct{})
		}
		file.incomplete[filename] = struct{}{}
		return errors.Join(closeErr, fmt.Errorf("roll back partial write: %w", err))
	}
	return closeErr
}

func (file *rollingFile) open(filename string) error {
	var size int64
	info, err := os.Lstat(filename)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			return err
		}
	case err != nil:
		return err
	case !info.Mode().IsRegular():
		return fmt.Errorf("capture path is not a regular file: %s", filename)
	default:
		if _, incomplete := file.incomplete[filename]; incomplete {
			if err := rotateFile(filename); err != nil {
				return fmt.Errorf("rotate file after failed rollback: %w", err)
			}
			info = nil
		} else {
			size = info.Size()
		}
	}
	delete(file.incomplete, filename)

	handle, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	if info != nil && info.Mode().Perm()&0077 != 0 {
		if err := handle.Chmod(0600); err != nil {
			return errors.Join(err, handle.Close())
		}
	}
	file.handle = handle
	file.size = size
	file.prunedBackups = -1
	return nil
}

func (file *rollingFile) close() error {
	if file.handle == nil {
		return nil
	}
	handle := file.handle
	file.handle = nil
	file.size = 0
	return handle.Close()
}

func rotateFile(filename string) error {
	prefix := filename + "." + time.Now().UTC().Format(backupTimeLayout)
	for sequence := 0; ; sequence++ {
		backup := fmt.Sprintf("%s-%010d.bak", prefix, sequence)
		_, err := os.Lstat(backup)
		if errors.Is(err, os.ErrNotExist) {
			return os.Rename(filename, backup)
		}
		if err != nil {
			return err
		}
	}
}

func pruneFileBackups(filename string, maxBackups int) error {
	if maxBackups == 0 {
		return nil
	}
	dir := filepath.Dir(filename)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	prefix := filepath.Base(filename) + "."
	kept := 0
	for _, entry := range slices.Backward(entries) {
		suffix, matches := strings.CutPrefix(entry.Name(), prefix)
		suffix, backup := strings.CutSuffix(suffix, ".bak")
		separator := strings.LastIndexByte(suffix, '-')
		if !matches || !backup || separator < 0 || len(suffix)-separator-1 != 10 || !entry.Type().IsRegular() {
			continue
		}
		stamp, sequence := suffix[:separator], suffix[separator+1:]
		if _, err := time.Parse(backupTimeLayout, stamp); err != nil {
			continue
		}
		if _, err := strconv.ParseUint(sequence, 10, 64); err != nil {
			continue
		}
		if kept < maxBackups {
			kept++
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
