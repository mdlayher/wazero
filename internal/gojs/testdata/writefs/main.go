package writefs

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"syscall"
	"time"
)

func Main() {
	// Create a test directory
	dir := path.Join(os.TempDir(), "dir")
	dir1 := path.Join(os.TempDir(), "dir1")
	err := os.Mkdir(dir, 0o700)
	if err != nil {
		log.Panicln(err)
		return
	}
	defer os.Remove(dir)

	// Create a test file in that directory
	file := path.Join(dir, "file")
	file1 := path.Join(os.TempDir(), "file1")
	err = os.WriteFile(file, []byte{}, 0o600)
	if err != nil {
		log.Panicln(err)
		return
	}
	defer os.Remove(file)

	// Ensure stat works, particularly mode.
	for _, path := range []string{dir, file} {
		if stat, err := os.Stat(path); err != nil {
			log.Panicln(err)
		} else {
			fmt.Println(path, "mode", stat.Mode())
		}
	}

	// Test removing a non-empty empty directory
	if err = syscall.Rmdir(dir); err != syscall.ENOTEMPTY {
		log.Panicln("unexpected error", err)
	}

	// Test updating the mod time of a file, noting JS has millis precision.
	atime := time.Unix(123, 4*1e6)
	mtime := time.Unix(567, 8*1e6)

	// Ensure errors propagate
	if err = os.Chtimes("noexist", atime, mtime); !errors.Is(err, syscall.ENOENT) {
		log.Panicln("unexpected error", err)
	}

	// Now, try a real update.
	if err = os.Chtimes(dir, atime, mtime); err != nil {
		log.Panicln("unexpected error", err)
	}

	// Ensure the times translated properly.
	if stat, err := os.Stat(dir); err != nil {
		log.Panicln("unexpected error", err)
	} else {
		atimeSec, atimeNsec, mtimeSec, mtimeNsec, _, _ := statTimes(stat)
		fmt.Println("times:", atimeSec, atimeNsec, mtimeSec, mtimeNsec)
	}

	// Test renaming a file, noting we can't verify error numbers as they
	// vary per operating system.
	if err = syscall.Rename(file, dir); err == nil {
		log.Panicln("expected error")
	}
	if err = syscall.Rename(file, file1); err != nil {
		log.Panicln("unexpected error", err)
	}

	// Test renaming a directory
	if err = syscall.Rename(dir, file1); err == nil {
		log.Panicln("expected error")
	}
	if err = syscall.Rename(dir, dir1); err != nil {
		log.Panicln("unexpected error", err)
	}

	// Test unlinking a file
	if err = syscall.Rmdir(file1); err != syscall.ENOTDIR {
		log.Panicln("unexpected error", err)
	}
	if err = syscall.Unlink(file1); err != nil {
		log.Panicln("unexpected error", err)
	}

	// Test removing an empty directory
	if err = syscall.Unlink(dir1); err != syscall.EISDIR {
		log.Panicln("unexpected error", err)
	}
	if err = syscall.Rmdir(dir1); err != nil {
		log.Panicln("unexpected error", err)
	}

	// shouldn't fail
	if err = os.RemoveAll(dir1); err != nil {
		log.Panicln(err)
		return
	}
}
