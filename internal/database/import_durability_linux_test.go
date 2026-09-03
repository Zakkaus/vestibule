//go:build linux && amd64

package database

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"unsafe"
)

const (
	seccompSetModeFilter          = 1
	seccompFilterFlagNewListener  = 8
	seccompReturnAllow            = 0x7fff0000
	seccompReturnUserNotification = 0x7fc00000
	seccompIOCTLNotificationRecv  = 0xc0502100
	seccompIOCTLNotificationSend  = 0xc0182101
	seccompNotificationContinue   = 1
	prSetNoNewPrivileges          = 38
	seccompSystemCall             = 317

	bpfLoadWordAbsolute = 0x20
	bpfJumpEqual        = 0x15
	bpfReturn           = 0x06
)

type pollDescriptor struct {
	fd      int32
	events  int16
	revents int16
}

type seccompData struct {
	Number             int32
	Architecture       uint32
	InstructionPointer uint64
	Arguments          [6]uint64
}

type seccompNotification struct {
	ID    uint64
	PID   uint32
	Flags uint32
	Data  seccompData
}

type seccompNotificationResponse struct {
	ID    uint64
	Value int64
	Error int32
	Flags uint32
}

type fsyncTrace struct {
	paths []string
	err   error
}

func TestBackupSyncsEachFileThenItsDirectoryAndParent(t *testing.T) {
	stateDirectory := copyLegacyFixtures(t)
	backupDirectory := filepath.Join(t.TempDir(), "parent", "backup")
	listener := installFsyncTrace(t)
	stop := make(chan struct{})
	traced := make(chan fsyncTrace, 1)
	go traceFsyncCalls(listener, stop, traced)

	_, backupErr := backupLegacyJSON(ImportOptions{
		StateDirectory:  stateDirectory,
		BackupDirectory: backupDirectory,
	})
	close(stop)
	trace := <-traced
	if err := syscall.Close(listener); err != nil {
		t.Errorf("close fsync trace listener: %v", err)
	}
	if trace.err != nil {
		t.Fatalf("trace backup durability calls: %v", trace.err)
	}
	if backupErr != nil {
		t.Fatalf("backup while tracing durability calls: %v", backupErr)
	}

	for i, name := range legacyJSONNames {
		path := filepath.Join(backupDirectory, name)
		if len(trace.paths) <= i || trace.paths[i] != path {
			t.Fatalf("backup did not fsync %s in file order before syncing directories; a power loss could erase the backup", name)
		}
	}
	directoryIndex := len(legacyJSONNames)
	if len(trace.paths) <= directoryIndex || trace.paths[directoryIndex] != backupDirectory {
		t.Fatal("backup did not fsync its directory after every file and before its parent")
	}
	if len(trace.paths) <= directoryIndex+1 || trace.paths[directoryIndex+1] != filepath.Dir(backupDirectory) {
		t.Fatal("backup did not fsync its parent after the backup directory")
	}
	if len(trace.paths) != directoryIndex+2 {
		t.Fatalf("backup made %d fsync calls, want exactly %d file and directory durability barriers", len(trace.paths), directoryIndex+2)
	}
}

func installFsyncTrace(t *testing.T) int {
	t.Helper()
	// Seccomp filters cannot be removed. Keeping this goroutine locked makes the runtime
	// retire its filtered thread when the test returns instead of reusing it for another test.
	runtime.LockOSThread()
	if _, _, errno := syscall.RawSyscall6(
		syscall.SYS_PRCTL, prSetNoNewPrivileges, 1, 0, 0, 0, 0,
	); errno != 0 {
		t.Fatalf("enable no-new-privileges for fsync trace: %v", errno)
	}
	filters := []syscall.SockFilter{
		{Code: bpfLoadWordAbsolute, K: 0},
		{Code: bpfJumpEqual, Jf: 1, K: uint32(syscall.SYS_FSYNC)},
		{Code: bpfReturn, K: seccompReturnUserNotification},
		{Code: bpfReturn, K: seccompReturnAllow},
	}
	program := syscall.SockFprog{Len: uint16(len(filters)), Filter: &filters[0]}
	fd, _, errno := syscall.RawSyscall(
		seccompSystemCall,
		seccompSetModeFilter,
		seccompFilterFlagNewListener,
		uintptr(unsafe.Pointer(&program)),
	)
	runtime.KeepAlive(filters)
	if errno != 0 {
		t.Fatalf("install fsync trace: %v", errno)
	}
	return int(fd)
}

func traceFsyncCalls(listener int, stop <-chan struct{}, result chan<- fsyncTrace) {
	trace := fsyncTrace{}
	for {
		select {
		case <-stop:
			result <- trace
			return
		default:
		}
		ready, err := waitForSeccompNotification(listener)
		if err != nil {
			trace.err = fmt.Errorf("wait for fsync notification: %w", err)
			result <- trace
			return
		}
		if !ready {
			continue
		}
		var notification seccompNotification
		err = seccompIOCTL(listener, seccompIOCTLNotificationRecv, unsafe.Pointer(&notification))
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			trace.err = fmt.Errorf("receive fsync notification: %w", err)
			result <- trace
			return
		}
		path, pathErr := os.Readlink(fmt.Sprintf(
			"/proc/self/task/%d/fd/%d", notification.PID, notification.Data.Arguments[0],
		))
		response := seccompNotificationResponse{
			ID:    notification.ID,
			Flags: seccompNotificationContinue,
		}
		responseErr := seccompIOCTL(listener, seccompIOCTLNotificationSend, unsafe.Pointer(&response))
		if pathErr != nil && trace.err == nil {
			trace.err = fmt.Errorf("identify fsync target: %w", pathErr)
		}
		if responseErr != nil {
			trace.err = fmt.Errorf("continue fsync: %w", responseErr)
			result <- trace
			return
		}
		trace.paths = append(trace.paths, path)
	}
}

func waitForSeccompNotification(fd int) (bool, error) {
	descriptor := pollDescriptor{fd: int32(fd), events: 1}
	count, _, errno := syscall.Syscall(
		syscall.SYS_POLL,
		uintptr(unsafe.Pointer(&descriptor)),
		1,
		10,
	)
	runtime.KeepAlive(&descriptor)
	if errno == syscall.EINTR {
		return false, nil
	}
	if errno != 0 {
		return false, errno
	}
	return count != 0 && descriptor.revents&1 != 0, nil
}

func seccompIOCTL(fd int, request uintptr, value unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), request, uintptr(value))
	if errno != 0 {
		return errno
	}
	return nil
}
