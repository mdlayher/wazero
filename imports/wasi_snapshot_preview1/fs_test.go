package wasi_snapshot_preview1_test

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/syscallfs"
	"github.com/tetratelabs/wazero/internal/testing/require"
	. "github.com/tetratelabs/wazero/internal/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// Test_fdAdvise only tests it is stubbed for GrainLang per #271
func Test_fdAdvise(t *testing.T) {
	log := requireErrnoNosys(t, FdAdviseName, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_advise(fd=0,offset=0,len=0,advice=0)
<-- errno=ENOSYS
`, log)
}

// Test_fdAllocate only tests it is stubbed for GrainLang per #271
func Test_fdAllocate(t *testing.T) {
	log := requireErrnoNosys(t, FdAllocateName, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_allocate(fd=0,offset=0,len=0)
<-- errno=ENOSYS
`, log)
}

func Test_fdClose(t *testing.T) {
	// fd_close needs to close an open file descriptor. Open two files so that we can tell which is closed.
	path1, path2 := "dir/-", "dir/a-"
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)

	// open both paths without using WASI
	fsc := mod.(*wasm.CallContext).Sys.FS()

	fdToClose, err := fsc.OpenFile(path1, os.O_RDONLY, 0)
	require.NoError(t, err)

	fdToKeep, err := fsc.OpenFile(path2, os.O_RDONLY, 0)
	require.NoError(t, err)

	// Close
	requireErrno(t, ErrnoSuccess, mod, FdCloseName, uint64(fdToClose))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_close(fd=4)
<== errno=ESUCCESS
`, "\n"+log.String())

	// Verify fdToClose is closed and removed from the opened FDs.
	_, ok := fsc.LookupFile(fdToClose)
	require.False(t, ok)

	// Verify fdToKeep is not closed
	_, ok = fsc.LookupFile(fdToKeep)
	require.True(t, ok)

	log.Reset()
	t.Run("ErrnoBadF for an invalid FD", func(t *testing.T) {
		requireErrno(t, ErrnoBadf, mod, FdCloseName, uint64(42)) // 42 is an arbitrary invalid FD
		require.Equal(t, `
==> wasi_snapshot_preview1.fd_close(fd=42)
<== errno=EBADF
`, "\n"+log.String())
	})
}

// Test_fdDatasync only tests it is stubbed for GrainLang per #271
func Test_fdDatasync(t *testing.T) {
	log := requireErrnoNosys(t, FdDatasyncName, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_datasync(fd=0)
<-- errno=ENOSYS
`, log)
}

func Test_fdFdstatGet(t *testing.T) {
	file, dir := "animals.txt", "sub"
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)
	memorySize := mod.Memory().Size()

	// open both paths without using WASI
	fsc := mod.(*wasm.CallContext).Sys.FS()

	fileFD, err := fsc.OpenFile(file, os.O_RDONLY, 0)
	require.NoError(t, err)

	dirFD, err := fsc.OpenFile(dir, os.O_RDONLY, 0)
	require.NoError(t, err)

	tests := []struct {
		name             string
		fd, resultFdstat uint32
		expectedMemory   []byte
		expectedErrno    Errno
		expectedLog      string
	}{
		{
			name: "stdin",
			fd:   sys.FdStdin,
			expectedMemory: []byte{
				1, 0, // fs_filetype
				0, 0, 0, 0, 0, 0, // fs_flags
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=0)
<== (stat={filetype=BLOCK_DEVICE,fdflags=,fs_rights_base=,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
		{
			name: "stdout",
			fd:   sys.FdStdout,
			expectedMemory: []byte{
				1, 0, // fs_filetype
				1, 0, 0, 0, 0, 0, // fs_flags
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=1)
<== (stat={filetype=BLOCK_DEVICE,fdflags=APPEND,fs_rights_base=,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
		{
			name: "stderr",
			fd:   sys.FdStderr,
			expectedMemory: []byte{
				1, 0, // fs_filetype
				1, 0, 0, 0, 0, 0, // fs_flags
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=2)
<== (stat={filetype=BLOCK_DEVICE,fdflags=APPEND,fs_rights_base=,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
		{
			name: "root",
			fd:   sys.FdPreopen,
			expectedMemory: []byte{
				3, 0, // fs_filetype
				0, 0, 0, 0, 0, 0, // fs_flags
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=3)
<== (stat={filetype=DIRECTORY,fdflags=,fs_rights_base=,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
		{
			name: "file",
			fd:   fileFD,
			expectedMemory: []byte{
				4, 0, // fs_filetype
				0, 0, 0, 0, 0, 0, // fs_flags
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=4)
<== (stat={filetype=REGULAR_FILE,fdflags=,fs_rights_base=,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
		{
			name: "dir",
			fd:   dirFD,
			expectedMemory: []byte{
				3, 0, // fs_filetype
				0, 0, 0, 0, 0, 0, // fs_flags
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=5)
<== (stat={filetype=DIRECTORY,fdflags=,fs_rights_base=,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
		{
			name:          "bad FD",
			fd:            math.MaxUint32,
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=-1)
<== (stat=,errno=EBADF)
`,
		},
		{
			name:          "resultFdstat exceeds the maximum valid address by 1",
			fd:            dirFD,
			resultFdstat:  memorySize - 24 + 1,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=5)
<== (stat=,errno=EFAULT)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, mod, len(tc.expectedMemory))

			requireErrno(t, tc.expectedErrno, mod, FdFdstatGetName, uint64(tc.fd), uint64(tc.resultFdstat))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

// Test_fdFdstatSetFlags only tests it is stubbed for GrainLang per #271
func Test_fdFdstatSetFlags(t *testing.T) {
	log := requireErrnoNosys(t, FdFdstatSetFlagsName, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_fdstat_set_flags(fd=0,flags=0)
<-- errno=ENOSYS
`, log)
}

// Test_fdFdstatSetRights only tests it is stubbed for GrainLang per #271
func Test_fdFdstatSetRights(t *testing.T) {
	log := requireErrnoNosys(t, FdFdstatSetRightsName, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_fdstat_set_rights(fd=0,fs_rights_base=,fs_rights_inheriting=)
<-- errno=ENOSYS
`, log)
}

func Test_fdFilestatGet(t *testing.T) {
	file, dir := "animals.txt", "sub"
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)
	memorySize := mod.Memory().Size()

	// open both paths without using WASI
	fsc := mod.(*wasm.CallContext).Sys.FS()

	fileFD, err := fsc.OpenFile(file, os.O_RDONLY, 0)
	require.NoError(t, err)

	dirFD, err := fsc.OpenFile(dir, os.O_RDONLY, 0)
	require.NoError(t, err)

	tests := []struct {
		name               string
		fd, resultFilestat uint32
		expectedMemory     []byte
		expectedErrno      Errno
		expectedLog        string
	}{
		{
			name: "stdin",
			fd:   sys.FdStdin,
			expectedMemory: []byte{
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				// expect block device because stdin isn't a real file
				1, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				0, 0, 0, 0, 0, 0, 0, 0, // size
				0, 0, 0, 0, 0, 0, 0, 0, // atim
				0, 0, 0, 0, 0, 0, 0, 0, // mtim
				0, 0, 0, 0, 0, 0, 0, 0, // ctim
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_get(fd=0)
<== (filestat={filetype=BLOCK_DEVICE,size=0,mtim=0},errno=ESUCCESS)
`,
		},
		{
			name: "stdout",
			fd:   sys.FdStdout,
			expectedMemory: []byte{
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				// expect block device because stdout isn't a real file
				1, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				0, 0, 0, 0, 0, 0, 0, 0, // size
				0, 0, 0, 0, 0, 0, 0, 0, // atim
				0, 0, 0, 0, 0, 0, 0, 0, // mtim
				0, 0, 0, 0, 0, 0, 0, 0, // ctim
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_get(fd=1)
<== (filestat={filetype=BLOCK_DEVICE,size=0,mtim=0},errno=ESUCCESS)
`,
		},
		{
			name: "stderr",
			fd:   sys.FdStderr,
			expectedMemory: []byte{
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				// expect block device because stderr isn't a real file
				1, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				0, 0, 0, 0, 0, 0, 0, 0, // size
				0, 0, 0, 0, 0, 0, 0, 0, // atim
				0, 0, 0, 0, 0, 0, 0, 0, // mtim
				0, 0, 0, 0, 0, 0, 0, 0, // ctim
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_get(fd=2)
<== (filestat={filetype=BLOCK_DEVICE,size=0,mtim=0},errno=ESUCCESS)
`,
		},
		{
			name: "root",
			fd:   sys.FdPreopen,
			expectedMemory: []byte{
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				3, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				0, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x0, 0x7c, 0x78, 0x9d, 0xf2, 0x55, 0x16, // atim
				0x0, 0x0, 0x7c, 0x78, 0x9d, 0xf2, 0x55, 0x16, // mtim
				0x0, 0x0, 0x7c, 0x78, 0x9d, 0xf2, 0x55, 0x16, // ctim
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_get(fd=3)
<== (filestat={filetype=DIRECTORY,size=0,mtim=1609459200000000000},errno=ESUCCESS)
`,
		},
		{
			name: "file",
			fd:   fileFD,
			expectedMemory: []byte{
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				4, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				30, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // atim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // mtim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // ctim
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_get(fd=4)
<== (filestat={filetype=REGULAR_FILE,size=30,mtim=1667482413000000000},errno=ESUCCESS)
`,
		},
		{
			name: "dir",
			fd:   dirFD,
			expectedMemory: []byte{
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				3, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				0, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // atim
				0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // mtim
				0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // ctim
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_get(fd=5)
<== (filestat={filetype=DIRECTORY,size=0,mtim=1640995200000000000},errno=ESUCCESS)
`,
		},
		{
			name:          "bad FD",
			fd:            math.MaxUint32,
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_get(fd=-1)
<== (filestat=,errno=EBADF)
`,
		},
		{
			name:           "resultFilestat exceeds the maximum valid address by 1",
			fd:             dirFD,
			resultFilestat: memorySize - 64 + 1,
			expectedErrno:  ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_get(fd=5)
<== (filestat=,errno=EFAULT)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, mod, len(tc.expectedMemory))

			requireErrno(t, tc.expectedErrno, mod, FdFilestatGetName, uint64(tc.fd), uint64(tc.resultFilestat))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

// Test_fdFilestatSetSize only tests it is stubbed for GrainLang per #271
func Test_fdFilestatSetSize(t *testing.T) {
	log := requireErrnoNosys(t, FdFilestatSetSizeName, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_filestat_set_size(fd=0,size=0)
<-- errno=ENOSYS
`, log)
}

// Test_fdFilestatSetTimes only tests it is stubbed for GrainLang per #271
func Test_fdFilestatSetTimes(t *testing.T) {
	log := requireErrnoNosys(t, FdFilestatSetTimesName, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_filestat_set_times(fd=0,atim=0,mtim=0,fst_flags=0)
<-- errno=ENOSYS
`, log)
}

func Test_fdPread(t *testing.T) {
	tmpDir := t.TempDir()
	mod, fd, log, r := requireOpenFile(t, tmpDir, "test_path", []byte("wazero"), true)
	defer r.Close(testCtx)

	iovs := uint32(1) // arbitrary offset
	initialMemory := []byte{
		'?',         // `iovs` is after this
		18, 0, 0, 0, // = iovs[0].offset
		4, 0, 0, 0, // = iovs[0].length
		23, 0, 0, 0, // = iovs[1].offset
		2, 0, 0, 0, // = iovs[1].length
		'?',
	}

	iovsCount := uint32(2)    // The count of iovs
	resultNread := uint32(26) // arbitrary offset

	tests := []struct {
		name           string
		offset         int64
		expectedMemory []byte
		expectedLog    string
	}{
		{
			name:   "offset zero",
			offset: 0,
			expectedMemory: append(
				initialMemory,
				'w', 'a', 'z', 'e', // iovs[0].length bytes
				'?',      // iovs[1].offset is after this
				'r', 'o', // iovs[1].length bytes
				'?',        // resultNread is after this
				6, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
				'?',
			),
			expectedLog: `
==> wasi_snapshot_preview1.fd_pread(fd=4,iovs=1,iovs_len=2,offset=0)
<== (nread=6,errno=ESUCCESS)
`,
		},
		{
			name:   "offset 2",
			offset: 2,
			expectedMemory: append(
				initialMemory,
				'z', 'e', 'r', 'o', // iovs[0].length bytes
				'?', '?', '?', '?', // resultNread is after this
				4, 0, 0, 0, // sum(iovs[...].length) == length of "zero"
				'?',
			),
			expectedLog: `
==> wasi_snapshot_preview1.fd_pread(fd=4,iovs=1,iovs_len=2,offset=2)
<== (nread=4,errno=ESUCCESS)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, mod, len(tc.expectedMemory))

			ok := mod.Memory().Write(0, initialMemory)
			require.True(t, ok)

			requireErrno(t, ErrnoSuccess, mod, FdPreadName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(tc.offset), uint64(resultNread))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

func Test_fdPread_offset(t *testing.T) {
	tmpDir := t.TempDir()
	mod, fd, log, r := requireOpenFile(t, tmpDir, "test_path", []byte("wazero"), true)
	defer r.Close(testCtx)

	// Do an initial fdPread.

	iovs := uint32(1) // arbitrary offset
	initialMemory := []byte{
		'?',         // `iovs` is after this
		18, 0, 0, 0, // = iovs[0].offset
		4, 0, 0, 0, // = iovs[0].length
		23, 0, 0, 0, // = iovs[1].offset
		2, 0, 0, 0, // = iovs[1].length
		'?',
	}
	iovsCount := uint32(2)    // The count of iovs
	resultNread := uint32(26) // arbitrary offset

	expectedMemory := append(
		initialMemory,
		'z', 'e', 'r', 'o', // iovs[0].length bytes
		'?', '?', '?', '?', // resultNread is after this
		4, 0, 0, 0, // sum(iovs[...].length) == length of "zero"
		'?',
	)

	maskMemory(t, mod, len(expectedMemory))

	ok := mod.Memory().Write(0, initialMemory)
	require.True(t, ok)

	requireErrno(t, ErrnoSuccess, mod, FdPreadName, uint64(fd), uint64(iovs), uint64(iovsCount), 2, uint64(resultNread))
	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)

	// Verify that the fdPread didn't affect the fdRead offset.

	expectedMemory = append(
		initialMemory,
		'w', 'a', 'z', 'e', // iovs[0].length bytes
		'?',      // iovs[1].offset is after this
		'r', 'o', // iovs[1].length bytes
		'?',        // resultNread is after this
		6, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
		'?',
	)

	requireErrno(t, ErrnoSuccess, mod, FdReadName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultNread))
	actual, ok = mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)

	expectedLog := `
==> wasi_snapshot_preview1.fd_pread(fd=4,iovs=1,iovs_len=2,offset=2)
<== (nread=4,errno=ESUCCESS)
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=1,iovs_len=2)
<== (nread=6,errno=ESUCCESS)
`
	require.Equal(t, expectedLog, "\n"+log.String())
}

func Test_fdPread_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	contents := []byte("wazero")
	mod, fd, log, r := requireOpenFile(t, tmpDir, "test_path", contents, true)
	defer r.Close(testCtx)

	tests := []struct {
		name                             string
		fd, iovs, iovsCount, resultNread uint32
		offset                           int64
		memory                           []byte
		expectedErrno                    Errno
		expectedLog                      string
	}{
		{
			name:          "invalid fd",
			fd:            42,                         // arbitrary invalid fd
			memory:        []byte{'?', '?', '?', '?'}, // pass result.nread validation
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pread(fd=42,iovs=65532,iovs_len=65532,offset=0)
<== (nread=,errno=EBADF)
`,
		},
		{
			name:          "seek past file",
			fd:            fd,
			offset:        int64(len(contents) + 1),
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pread(fd=4,iovs=65536,iovs_len=65536,offset=7)
<== (nread=,errno=EFAULT)
`,
		},
		{
			name:          "out-of-memory reading iovs[0].offset",
			fd:            fd,
			iovs:          1,
			memory:        []byte{'?'},
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pread(fd=4,iovs=65536,iovs_len=65535,offset=0)
<== (nread=,errno=EFAULT)
`,
		},
		{
			name: "out-of-memory reading iovs[0].length",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
			},
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pread(fd=4,iovs=65532,iovs_len=65532,offset=0)
<== (nread=,errno=EFAULT)
`,
		},
		{
			name: "iovs[0].offset is outside memory",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			memory: []byte{
				'?',          // `iovs` is after this
				0, 0, 0x1, 0, // = iovs[0].offset on the second page
				1, 0, 0, 0, // = iovs[0].length
			},
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pread(fd=4,iovs=65528,iovs_len=65528,offset=0)
<== (nread=,errno=EFAULT)
`,
		},
		{
			name: "length to read exceeds memory by 1",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
				0, 0, 0x1, 0, // = iovs[0].length on the second page
				'?',
			},
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pread(fd=4,iovs=65527,iovs_len=65527,offset=0)
<== (nread=,errno=EFAULT)
`,
		},
		{
			name: "resultNread offset is outside memory",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			resultNread: 10, // 1 past memory
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
				1, 0, 0, 0, // = iovs[0].length
				'?',
			},
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pread(fd=4,iovs=65527,iovs_len=65527,offset=0)
<== (nread=,errno=EFAULT)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			offset := uint32(wasm.MemoryPagesToBytesNum(testMemoryPageSize) - uint64(len(tc.memory)))

			memoryWriteOK := mod.Memory().Write(offset, tc.memory)
			require.True(t, memoryWriteOK)

			requireErrno(t, tc.expectedErrno, mod, FdPreadName, uint64(tc.fd), uint64(tc.iovs+offset), uint64(tc.iovsCount+offset), uint64(tc.offset), uint64(tc.resultNread+offset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdPrestatGet(t *testing.T) {
	testfs, err := syscallfs.NewDirFS(t.TempDir())
	require.NoError(t, err)

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(testfs))
	defer r.Close(testCtx)
	dirFD := sys.FdPreopen

	resultPrestat := uint32(1) // arbitrary offset
	expectedMemory := []byte{
		'?',     // resultPrestat after this
		0,       // 8-bit tag indicating `prestat_dir`, the only available tag
		0, 0, 0, // 3-byte padding
		// the result path length field after this
		1, 0, 0, 0, // = in little endian encoding
		'?',
	}

	maskMemory(t, mod, len(expectedMemory))

	requireErrno(t, ErrnoSuccess, mod, FdPrestatGetName, uint64(dirFD), uint64(resultPrestat))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_prestat_get(fd=3)
<== (prestat={pr_name_len=1},errno=ESUCCESS)
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_fdPrestatGet_Errors(t *testing.T) {
	mod, dirFD, log, r := requireOpenFile(t, t.TempDir(), "tmp", nil, true)
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size()
	tests := []struct {
		name          string
		fd            uint32
		resultPrestat uint32
		expectedErrno Errno
		expectedLog   string
	}{
		{
			name:          "unopened FD",
			fd:            42, // arbitrary invalid FD
			resultPrestat: 0,  // valid offset
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_get(fd=42)
<== (prestat=,errno=EBADF)
`,
		},
		{
			name:          "not pre-opened FD",
			fd:            dirFD,
			resultPrestat: 0, // valid offset
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_get(fd=4)
<== (prestat=,errno=EINVAL)
`,
		},
		{
			name:          "out-of-memory resultPrestat",
			fd:            sys.FdPreopen,
			resultPrestat: memorySize,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_get(fd=3)
<== (prestat=,errno=EFAULT)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, tc.expectedErrno, mod, FdPrestatGetName, uint64(tc.fd), uint64(tc.resultPrestat))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdPrestatDirName(t *testing.T) {
	testfs, err := syscallfs.NewDirFS(t.TempDir())
	require.NoError(t, err)

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(testfs))
	defer r.Close(testCtx)
	dirFD := sys.FdPreopen

	path := uint32(1)    // arbitrary offset
	pathLen := uint32(0) // shorter than len("/") to prove truncation is ok
	expectedMemory := []byte{
		'?', '?', '?', '?',
	}

	maskMemory(t, mod, len(expectedMemory))

	requireErrno(t, ErrnoSuccess, mod, FdPrestatDirNameName, uint64(dirFD), uint64(path), uint64(pathLen))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=3)
<== (path=,errno=ESUCCESS)
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_fdPrestatDirName_Errors(t *testing.T) {
	mod, dirFD, log, r := requireOpenFile(t, t.TempDir(), "tmp", nil, true)
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size()
	maskMemory(t, mod, 10)

	validAddress := uint32(0) // Arbitrary valid address as arguments to fd_prestat_dir_name. We chose 0 here.
	pathLen := uint32(len("/"))

	tests := []struct {
		name          string
		fd            uint32
		path          uint32
		pathLen       uint32
		expectedErrno Errno
		expectedLog   string
	}{
		{
			name:          "out-of-memory path",
			fd:            sys.FdPreopen,
			path:          memorySize,
			pathLen:       pathLen,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=3)
<== (path=,errno=EFAULT)
`,
		},
		{
			name:          "path exceeds the maximum valid address by 1",
			fd:            sys.FdPreopen,
			path:          memorySize - pathLen + 1,
			pathLen:       pathLen,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=3)
<== (path=,errno=EFAULT)
`,
		},
		{
			name:          "pathLen exceeds the length of the dir name",
			fd:            sys.FdPreopen,
			path:          validAddress,
			pathLen:       pathLen + 1,
			expectedErrno: ErrnoNametoolong,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=3)
<== (path=,errno=ENAMETOOLONG)
`,
		},
		{
			name:          "unopened FD",
			fd:            42, // arbitrary invalid fd
			path:          validAddress,
			pathLen:       pathLen,
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=42)
<== (path=,errno=EBADF)
`,
		},
		{
			name:          "not pre-opened FD",
			fd:            dirFD,
			path:          validAddress,
			pathLen:       pathLen,
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=4)
<== (path=,errno=EINVAL)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, tc.expectedErrno, mod, FdPrestatDirNameName, uint64(tc.fd), uint64(tc.path), uint64(tc.pathLen))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

// Test_fdPwrite only tests it is stubbed for GrainLang per #271
func Test_fdPwrite(t *testing.T) {
	log := requireErrnoNosys(t, FdPwriteName, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_pwrite(fd=0,iovs=0,iovs_len=0,offset=0)
<-- (nwritten=,errno=ENOSYS)
`, log)
}

func Test_fdRead(t *testing.T) {
	mod, fd, log, r := requireOpenFile(t, t.TempDir(), "test_path", []byte("wazero"), true)
	defer r.Close(testCtx)

	iovs := uint32(1) // arbitrary offset
	initialMemory := []byte{
		'?',         // `iovs` is after this
		18, 0, 0, 0, // = iovs[0].offset
		4, 0, 0, 0, // = iovs[0].length
		23, 0, 0, 0, // = iovs[1].offset
		2, 0, 0, 0, // = iovs[1].length
		'?',
	}
	iovsCount := uint32(2)    // The count of iovs
	resultNread := uint32(26) // arbitrary offset
	expectedMemory := append(
		initialMemory,
		'w', 'a', 'z', 'e', // iovs[0].length bytes
		'?',      // iovs[1].offset is after this
		'r', 'o', // iovs[1].length bytes
		'?',        // resultNread is after this
		6, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
		'?',
	)

	maskMemory(t, mod, len(expectedMemory))

	ok := mod.Memory().Write(0, initialMemory)
	require.True(t, ok)

	requireErrno(t, ErrnoSuccess, mod, FdReadName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultNread))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=1,iovs_len=2)
<== (nread=6,errno=ESUCCESS)
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_fdRead_Errors(t *testing.T) {
	mod, fd, log, r := requireOpenFile(t, t.TempDir(), "test_path", []byte("wazero"), true)
	defer r.Close(testCtx)

	tests := []struct {
		name                             string
		fd, iovs, iovsCount, resultNread uint32
		memory                           []byte
		expectedErrno                    Errno
		expectedLog                      string
	}{
		{
			name:          "invalid fd",
			fd:            42,                         // arbitrary invalid fd
			memory:        []byte{'?', '?', '?', '?'}, // pass result.nread validation
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_read(fd=42,iovs=65532,iovs_len=65532)
<== (nread=,errno=EBADF)
`,
		},
		{
			name:          "out-of-memory reading iovs[0].offset",
			fd:            fd,
			iovs:          1,
			memory:        []byte{'?'},
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=65536,iovs_len=65535)
<== (nread=,errno=EFAULT)
`,
		},
		{
			name: "out-of-memory reading iovs[0].length",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
			},
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=65532,iovs_len=65532)
<== (nread=,errno=EFAULT)
`,
		},
		{
			name: "iovs[0].offset is outside memory",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			memory: []byte{
				'?',          // `iovs` is after this
				0, 0, 0x1, 0, // = iovs[0].offset on the second page
				1, 0, 0, 0, // = iovs[0].length
			},
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=65528,iovs_len=65528)
<== (nread=,errno=EFAULT)
`,
		},
		{
			name: "length to read exceeds memory by 1",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
				0, 0, 0x1, 0, // = iovs[0].length on the second page
				'?',
			},
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=65527,iovs_len=65527)
<== (nread=,errno=EFAULT)
`,
		},
		{
			name: "resultNread offset is outside memory",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			resultNread: 10, // 1 past memory
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
				1, 0, 0, 0, // = iovs[0].length
				'?',
			},
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=65527,iovs_len=65527)
<== (nread=,errno=EFAULT)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			offset := uint32(wasm.MemoryPagesToBytesNum(testMemoryPageSize) - uint64(len(tc.memory)))

			memoryWriteOK := mod.Memory().Write(offset, tc.memory)
			require.True(t, memoryWriteOK)

			requireErrno(t, tc.expectedErrno, mod, FdReadName, uint64(tc.fd), uint64(tc.iovs+offset), uint64(tc.iovsCount+offset), uint64(tc.resultNread+offset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

var (
	testDirEntries = func() []fs.DirEntry {
		entries, err := fstest.FS.ReadDir("dir")
		if err != nil {
			panic(err)
		}
		return entries
	}()

	dirent1 = []byte{
		1, 0, 0, 0, 0, 0, 0, 0, // d_next = 1
		0, 0, 0, 0, 0, 0, 0, 0, // d_ino = 0
		1, 0, 0, 0, // d_namlen = 1 character
		4, 0, 0, 0, // d_type = regular_file
		'-', // name
	}
	dirent2 = []byte{
		2, 0, 0, 0, 0, 0, 0, 0, // d_next = 2
		0, 0, 0, 0, 0, 0, 0, 0, // d_ino = 0
		2, 0, 0, 0, // d_namlen = 1 character
		3, 0, 0, 0, // d_type =  directory
		'a', '-', // name
	}
	dirent3 = []byte{
		3, 0, 0, 0, 0, 0, 0, 0, // d_next = 3
		0, 0, 0, 0, 0, 0, 0, 0, // d_ino = 0
		3, 0, 0, 0, // d_namlen = 3 characters
		4, 0, 0, 0, // d_type = regular_file
		'a', 'b', '-', // name
	}
)

func Test_fdReaddir(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)

	fsc := mod.(*wasm.CallContext).Sys.FS()

	fd, err := fsc.OpenFile("dir", os.O_RDONLY, 0)
	require.NoError(t, err)

	tests := []struct {
		name            string
		dir             func() *sys.FileEntry
		bufLen          uint32
		cookie          int64
		expectedMem     []byte
		expectedMemSize int
		expectedBufused uint32
		expectedReadDir *sys.ReadDir
	}{
		{
			name: "empty dir",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("emptydir")
				require.NoError(t, err)

				return &sys.FileEntry{File: dir}
			},
			bufLen:          DirentSize,
			cookie:          0,
			expectedBufused: 0,
			expectedMem:     []byte{},
			expectedReadDir: &sys.ReadDir{},
		},
		{
			name: "full read",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)

				return &sys.FileEntry{File: dir}
			},
			bufLen:          4096,
			cookie:          0,
			expectedBufused: 78, // length of all entries
			expectedMem:     append(append(dirent1, dirent2...), dirent3...),
			expectedReadDir: &sys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries,
			},
		},
		{
			name: "can't read name",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)

				return &sys.FileEntry{File: dir}
			},
			bufLen:          24, // length is long enough for first, but not the name.
			cookie:          0,
			expectedBufused: 24,           // == bufLen which is the size of the dirent
			expectedMem:     dirent1[:24], // header without name
			expectedReadDir: &sys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries,
			},
		},
		{
			name: "read exactly first",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)

				return &sys.FileEntry{File: dir}
			},
			bufLen:          25, // length is long enough for first + the name, but not more.
			cookie:          0,
			expectedBufused: 25, // length to read exactly first.
			expectedMem:     dirent1,
			expectedReadDir: &sys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries,
			},
		},
		{
			name: "read exactly second",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)
				entry, err := dir.(fs.ReadDirFile).ReadDir(1)
				require.NoError(t, err)

				return &sys.FileEntry{
					File: dir,
					ReadDir: &sys.ReadDir{
						CountRead: 1,
						Entries:   entry,
					},
				}
			},
			bufLen:          26, // length is long enough for exactly second.
			cookie:          1,  // d_next of first
			expectedBufused: 26, // length to read exactly second.
			expectedMem:     dirent2,
			expectedReadDir: &sys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries[1:],
			},
		},
		{
			name: "read second and a little more",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)
				entry, err := dir.(fs.ReadDirFile).ReadDir(1)
				require.NoError(t, err)

				return &sys.FileEntry{
					File: dir,
					ReadDir: &sys.ReadDir{
						CountRead: 1,
						Entries:   entry,
					},
				}
			},
			bufLen:          30, // length is longer than the second entry, but not long enough for a header.
			cookie:          1,  // d_next of first
			expectedBufused: 30, // length to read some more, but not enough for a header, so buf was exhausted.
			expectedMem:     dirent2,
			expectedMemSize: len(dirent2), // we do not want to compare the full buffer since we don't know what the leftover 4 bytes will contain.
			expectedReadDir: &sys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries[1:],
			},
		},
		{
			name: "read second and header of third",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)
				entry, err := dir.(fs.ReadDirFile).ReadDir(1)
				require.NoError(t, err)

				return &sys.FileEntry{
					File: dir,
					ReadDir: &sys.ReadDir{
						CountRead: 1,
						Entries:   entry,
					},
				}
			},
			bufLen:          50, // length is longer than the second entry + enough for the header of third.
			cookie:          1,  // d_next of first
			expectedBufused: 50, // length to read exactly second and the header of third.
			expectedMem:     append(dirent2, dirent3[0:24]...),
			expectedReadDir: &sys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries[1:],
			},
		},
		{
			name: "read second and third",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)
				entry, err := dir.(fs.ReadDirFile).ReadDir(1)
				require.NoError(t, err)

				return &sys.FileEntry{
					File: dir,
					ReadDir: &sys.ReadDir{
						CountRead: 1,
						Entries:   entry,
					},
				}
			},
			bufLen:          53, // length is long enough for second and third.
			cookie:          1,  // d_next of first
			expectedBufused: 53, // length to read exactly one second and third.
			expectedMem:     append(dirent2, dirent3...),
			expectedReadDir: &sys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries[1:],
			},
		},
		{
			name: "read exactly third",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)
				two, err := dir.(fs.ReadDirFile).ReadDir(2)
				require.NoError(t, err)

				return &sys.FileEntry{
					File: dir,
					ReadDir: &sys.ReadDir{
						CountRead: 2,
						Entries:   two[1:],
					},
				}
			},
			bufLen:          27, // length is long enough for exactly third.
			cookie:          2,  // d_next of second.
			expectedBufused: 27, // length to read exactly third.
			expectedMem:     dirent3,
			expectedReadDir: &sys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries[2:],
			},
		},
		{
			name: "read third and beyond",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)
				two, err := dir.(fs.ReadDirFile).ReadDir(2)
				require.NoError(t, err)

				return &sys.FileEntry{
					File: dir,
					ReadDir: &sys.ReadDir{
						CountRead: 2,
						Entries:   two[1:],
					},
				}
			},
			bufLen:          100, // length is long enough for third and more, but there is nothing more.
			cookie:          2,   // d_next of second.
			expectedBufused: 27,  // length to read exactly third.
			expectedMem:     dirent3,
			expectedReadDir: &sys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries[2:],
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			// Assign the state we are testing
			file, ok := fsc.LookupFile(fd)
			require.True(t, ok)
			dir := tc.dir()
			defer dir.File.Close()

			file.File = dir.File
			file.ReadDir = dir.ReadDir

			maskMemory(t, mod, int(tc.bufLen))

			resultBufused := uint32(0) // where to write the amount used out of bufLen
			buf := uint32(8)           // where to start the dirents
			requireErrno(t, ErrnoSuccess, mod, FdReaddirName,
				uint64(fd), uint64(buf), uint64(tc.bufLen), uint64(tc.cookie), uint64(resultBufused))

			// read back the bufused and compare memory against it
			bufUsed, ok := mod.Memory().ReadUint32Le(resultBufused)
			require.True(t, ok)
			require.Equal(t, tc.expectedBufused, bufUsed)

			mem, ok := mod.Memory().Read(buf, bufUsed)
			require.True(t, ok)

			if tc.expectedMem != nil {
				if tc.expectedMemSize == 0 {
					tc.expectedMemSize = len(tc.expectedMem)
				}
				require.Equal(t, tc.expectedMem, mem[:tc.expectedMemSize])
			}

			require.Equal(t, tc.expectedReadDir, file.ReadDir)
		})
	}
}

func Test_fdReaddir_Errors(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)
	memLen := mod.Memory().Size()

	fsc := mod.(*wasm.CallContext).Sys.FS()

	fileFD, err := fsc.OpenFile("animals.txt", os.O_RDONLY, 0)
	require.NoError(t, err)

	dirFD, err := fsc.OpenFile("dir", os.O_RDONLY, 0)
	require.NoError(t, err)

	tests := []struct {
		name                           string
		dir                            func() *sys.FileEntry
		fd, buf, bufLen, resultBufused uint32
		cookie                         int64
		readDir                        *sys.ReadDir
		expectedErrno                  Errno
		expectedLog                    string
	}{
		{
			name:          "out-of-memory reading buf",
			fd:            dirFD,
			buf:           memLen,
			bufLen:        1000,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=5,buf=65536,buf_len=1000,cookie=0,result.bufused=0)
<== errno=EFAULT
`,
		},
		{
			name: "invalid fd",
			fd:   42,                    // arbitrary invalid fd
			buf:  0, bufLen: DirentSize, // enough to read the dirent
			resultBufused: 1000, // arbitrary
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=42,buf=0,buf_len=24,cookie=0,result.bufused=1000)
<== errno=EBADF
`,
		},
		{
			name: "not a dir",
			fd:   fileFD,
			buf:  0, bufLen: DirentSize, // enough to read the dirent
			resultBufused: 1000, // arbitrary
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=4,buf=0,buf_len=24,cookie=0,result.bufused=1000)
<== errno=EBADF
`,
		},
		{
			name:          "out-of-memory reading bufLen",
			fd:            dirFD,
			buf:           memLen - 1,
			bufLen:        1000,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=5,buf=65535,buf_len=1000,cookie=0,result.bufused=0)
<== errno=EFAULT
`,
		},
		{
			name: "bufLen must be enough to write a struct",
			fd:   dirFD,
			buf:  0, bufLen: 1,
			resultBufused: 1000,
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=5,buf=0,buf_len=1,cookie=0,result.bufused=1000)
<== errno=EINVAL
`,
		},
		{
			name: "cookie invalid when no prior state",
			fd:   dirFD,
			buf:  0, bufLen: 1000,
			cookie:        1,
			resultBufused: 2000,
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=5,buf=0,buf_len=1000,cookie=1,result.bufused=2000)
<== errno=EINVAL
`,
		},
		{
			name: "negative cookie invalid",
			fd:   dirFD,
			buf:  0, bufLen: 1000,
			cookie:        -1,
			readDir:       &sys.ReadDir{CountRead: 1},
			resultBufused: 2000,
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=5,buf=0,buf_len=1000,cookie=-1,result.bufused=2000)
<== errno=EINVAL
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			// Reset the directory so that tests don't taint each other.
			if file, ok := fsc.LookupFile(tc.fd); ok && tc.fd == dirFD {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)
				defer dir.Close()

				file.File = dir
				file.ReadDir = nil
			}

			requireErrno(t, tc.expectedErrno, mod, FdReaddirName,
				uint64(tc.fd), uint64(tc.buf), uint64(tc.bufLen), uint64(tc.cookie), uint64(tc.resultBufused))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

// Test_fdRenumber only tests it is stubbed for GrainLang per #271
func Test_fdRenumber(t *testing.T) {
	log := requireErrnoNosys(t, FdRenumberName, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_renumber(fd=0,to=0)
<-- errno=ENOSYS
`, log)
}

func Test_fdSeek(t *testing.T) {
	mod, fd, log, r := requireOpenFile(t, t.TempDir(), "test_path", []byte("wazero"), true)
	defer r.Close(testCtx)

	resultNewoffset := uint32(1) // arbitrary offset in api.Memory for the new offset value

	tests := []struct {
		name           string
		offset         int64
		whence         int
		expectedOffset int64
		expectedMemory []byte
		expectedLog    string
	}{
		{
			name:           "SeekStart",
			offset:         4, // arbitrary offset
			whence:         io.SeekStart,
			expectedOffset: 4, // = offset
			expectedMemory: []byte{
				'?',                    // resultNewoffset is after this
				4, 0, 0, 0, 0, 0, 0, 0, // = expectedOffset
				'?',
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=4,whence=0,result.newoffset=1)
<== errno=ESUCCESS
`,
		},
		{
			name:           "SeekCurrent",
			offset:         1, // arbitrary offset
			whence:         io.SeekCurrent,
			expectedOffset: 2, // = 1 (the initial offset of the test file) + 1 (offset)
			expectedMemory: []byte{
				'?',                    // resultNewoffset is after this
				2, 0, 0, 0, 0, 0, 0, 0, // = expectedOffset
				'?',
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=1,whence=1,result.newoffset=1)
<== errno=ESUCCESS
`,
		},
		{
			name:           "SeekEnd",
			offset:         -1, // arbitrary offset, note that offset can be negative
			whence:         io.SeekEnd,
			expectedOffset: 5, // = 6 (the size of the test file with content "wazero") + -1 (offset)
			expectedMemory: []byte{
				'?',                    // resultNewoffset is after this
				5, 0, 0, 0, 0, 0, 0, 0, // = expectedOffset
				'?',
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=-1,whence=2,result.newoffset=1)
<== errno=ESUCCESS
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, mod, len(tc.expectedMemory))

			// Since we initialized this file, we know it is a seeker (because it is a MapFile)
			fsc := mod.(*wasm.CallContext).Sys.FS()
			f, ok := fsc.LookupFile(fd)
			require.True(t, ok)
			seeker := f.File.(io.Seeker)

			// set the initial offset of the file to 1
			offset, err := seeker.Seek(1, io.SeekStart)
			require.NoError(t, err)
			require.Equal(t, int64(1), offset)

			requireErrno(t, ErrnoSuccess, mod, FdSeekName, uint64(fd), uint64(tc.offset), uint64(tc.whence), uint64(resultNewoffset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)

			offset, err = seeker.Seek(0, io.SeekCurrent)
			require.NoError(t, err)
			require.Equal(t, tc.expectedOffset, offset) // test that the offset of file is actually updated.
		})
	}
}

func Test_fdSeek_Errors(t *testing.T) {
	mod, fd, log, r := requireOpenFile(t, t.TempDir(), "test_path", []byte("wazero"), true)
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size()

	tests := []struct {
		name                    string
		fd                      uint32
		offset                  uint64
		whence, resultNewoffset uint32
		expectedErrno           Errno
		expectedLog             string
	}{
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=42,offset=0,whence=0,result.newoffset=0)
<== errno=EBADF
`,
		},
		{
			name:          "invalid whence",
			fd:            fd,
			whence:        3, // invalid whence, the largest whence io.SeekEnd(2) + 1
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=0,whence=3,result.newoffset=0)
<== errno=EINVAL
`,
		},
		{
			name:            "out-of-memory writing resultNewoffset",
			fd:              fd,
			resultNewoffset: memorySize,
			expectedErrno:   ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=0,whence=0,result.newoffset=65536)
<== errno=EFAULT
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, tc.expectedErrno, mod, FdSeekName, uint64(tc.fd), tc.offset, uint64(tc.whence), uint64(tc.resultNewoffset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

// Test_fdSync only tests it is stubbed for GrainLang per #271
func Test_fdSync(t *testing.T) {
	log := requireErrnoNosys(t, FdSyncName, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_sync(fd=0)
<-- errno=ENOSYS
`, log)
}

// Test_fdTell only tests it is stubbed for GrainLang per #271
func Test_fdTell(t *testing.T) {
	log := requireErrnoNosys(t, FdTellName, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_tell(fd=0,result.offset=0)
<-- errno=ENOSYS
`, log)
}

func Test_fdWrite(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	pathName := "test_path"
	mod, fd, log, r := requireOpenFile(t, tmpDir, pathName, []byte{}, false)
	defer r.Close(testCtx)

	iovs := uint32(1) // arbitrary offset
	initialMemory := []byte{
		'?',         // `iovs` is after this
		18, 0, 0, 0, // = iovs[0].offset
		4, 0, 0, 0, // = iovs[0].length
		23, 0, 0, 0, // = iovs[1].offset
		2, 0, 0, 0, // = iovs[1].length
		'?',                // iovs[0].offset is after this
		'w', 'a', 'z', 'e', // iovs[0].length bytes
		'?',      // iovs[1].offset is after this
		'r', 'o', // iovs[1].length bytes
		'?',
	}
	iovsCount := uint32(2)       // The count of iovs
	resultNwritten := uint32(26) // arbitrary offset
	expectedMemory := append(
		initialMemory,
		6, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
		'?',
	)

	maskMemory(t, mod, len(expectedMemory))
	ok := mod.Memory().Write(0, initialMemory)
	require.True(t, ok)

	requireErrno(t, ErrnoSuccess, mod, FdWriteName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultNwritten))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=1,iovs_len=2)
<== (nwritten=6,errno=ESUCCESS)
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)

	// Since we initialized this file, we know we can read it by path
	buf, err := os.ReadFile(path.Join(tmpDir, pathName))
	require.NoError(t, err)

	require.Equal(t, []byte("wazero"), buf) // verify the file was actually written
}

// Test_fdWrite_discard ensures default configuration doesn't add needless
// overhead, but still returns valid data. For example, writing to STDOUT when
// it is io.Discard.
func Test_fdWrite_discard(t *testing.T) {
	// Default has io.Discard as stdout/stderr
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig())
	defer r.Close(testCtx)

	iovs := uint32(1) // arbitrary offset
	initialMemory := []byte{
		'?',         // `iovs` is after this
		18, 0, 0, 0, // = iovs[0].offset
		4, 0, 0, 0, // = iovs[0].length
		23, 0, 0, 0, // = iovs[1].offset
		2, 0, 0, 0, // = iovs[1].length
		'?',                // iovs[0].offset is after this
		'w', 'a', 'z', 'e', // iovs[0].length bytes
		'?',      // iovs[1].offset is after this
		'r', 'o', // iovs[1].length bytes
		'?',
	}
	iovsCount := uint32(2)       // The count of iovs
	resultNwritten := uint32(26) // arbitrary offset
	expectedMemory := append(
		initialMemory,
		6, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
		'?',
	)

	maskMemory(t, mod, len(expectedMemory))
	ok := mod.Memory().Write(0, initialMemory)
	require.True(t, ok)

	fd := sys.FdStdout
	requireErrno(t, ErrnoSuccess, mod, FdWriteName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultNwritten))
	// Should not amplify logging
	require.Zero(t, len(log.Bytes()))

	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_fdWrite_Errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	pathName := "test_path"
	mod, fd, log, r := requireOpenFile(t, tmpDir, pathName, nil, false)
	defer r.Close(testCtx)

	// Setup valid test memory
	iovsCount := uint32(1)
	memSize := mod.Memory().Size()

	tests := []struct {
		name                     string
		fd, iovs, resultNwritten uint32
		expectedErrno            Errno
		expectedLog              string
	}{
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=42,iovs=0,iovs_len=1)
<== (nwritten=,errno=EBADF)
`,
		},
		{
			name:          "out-of-memory reading iovs[0].offset",
			fd:            fd,
			iovs:          memSize - 2,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=65534,iovs_len=1)
<== (nwritten=,errno=EFAULT)
`,
		},
		{
			name:          "out-of-memory reading iovs[0].length",
			fd:            fd,
			iovs:          memSize - 4, // iovs[0].offset was 4 bytes and iovs[0].length next, but not enough mod.Memory()!
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=65532,iovs_len=1)
<== (nwritten=,errno=EFAULT)
`,
		},
		{
			name:          "iovs[0].offset is outside memory",
			fd:            fd,
			iovs:          memSize - 5, // iovs[0].offset (where to read "hi") is outside memory.
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=65531,iovs_len=1)
<== (nwritten=,errno=EFAULT)
`,
		},
		{
			name:          "length to read exceeds memory by 1",
			fd:            fd,
			iovs:          memSize - 9, // iovs[0].offset (where to read "hi") is in memory, but truncated.
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=65527,iovs_len=1)
<== (nwritten=,errno=EFAULT)
`,
		},
		{
			name:           "resultNwritten offset is outside memory",
			fd:             fd,
			resultNwritten: memSize, // read was ok, but there wasn't enough memory to write the result.
			expectedErrno:  ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=0,iovs_len=1)
<== (nwritten=,errno=EFAULT)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			mod.Memory().Write(tc.iovs, append(
				leb128.EncodeUint32(tc.iovs+8), // = iovs[0].offset (where the data "hi" begins)
				// = iovs[0].length (how many bytes are in "hi")
				2, 0, 0, 0,
				'h', 'i', // iovs[0].length bytes
			))

			requireErrno(t, tc.expectedErrno, mod, FdWriteName, uint64(tc.fd), uint64(tc.iovs), uint64(iovsCount),
				uint64(tc.resultNwritten))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_pathCreateDirectory(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fs, err := syscallfs.NewDirFS(tmpDir)
	require.NoError(t, err)

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fs))
	defer r.Close(testCtx)

	// set up the initial memory to include the path name starting at an offset.
	pathName := "wazero"
	realPath := path.Join(tmpDir, pathName)
	ok := mod.Memory().Write(0, append([]byte{'?'}, pathName...))
	require.True(t, ok)

	preopenedFD := sys.FdPreopen
	name := 1
	nameLen := len(pathName)

	requireErrno(t, ErrnoSuccess, mod, PathCreateDirectoryName, uint64(preopenedFD), uint64(name), uint64(nameLen))
	require.Equal(t, `
==> wasi_snapshot_preview1.path_create_directory(fd=3,path=wazero)
<== errno=ESUCCESS
`, "\n"+log.String())

	// ensure the directory was created
	stat, err := os.Stat(realPath)
	require.NoError(t, err)
	require.True(t, stat.IsDir())
	require.Equal(t, pathName, stat.Name())
}

func Test_pathCreateDirectory_Errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fs, err := syscallfs.NewDirFS(tmpDir)
	require.NoError(t, err)

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fs))
	defer r.Close(testCtx)

	file := "file"
	err = os.WriteFile(path.Join(tmpDir, file), []byte{}, 0o700)
	require.NoError(t, err)
	fileFD := requireOpenFD(t, mod, file)

	dir := "dir"
	err = os.Mkdir(path.Join(tmpDir, dir), 0o700)
	require.NoError(t, err)

	tests := []struct {
		name, pathName    string
		fd, path, pathLen uint32
		expectedErrno     Errno
		expectedLog       string
	}{
		{
			name:          "unopened FD",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_create_directory(fd=42,path=)
<== errno=EBADF
`,
		},
		{
			name:          "FD not a directory",
			fd:            fileFD,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_create_directory(fd=4,path=file)
<== errno=ENOTDIR
`,
		},
		{
			name:          "out-of-memory reading path",
			fd:            sys.FdPreopen,
			path:          mod.Memory().Size(),
			pathLen:       1,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_create_directory(fd=3,path=OOM(65536,1))
<== errno=EFAULT
`,
		},
		{
			name:          "out-of-memory reading pathLen",
			fd:            sys.FdPreopen,
			path:          0,
			pathLen:       mod.Memory().Size() + 1, // path is in the valid memory range, but pathLen is OOM for path
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_create_directory(fd=3,path=OOM(0,65537))
<== errno=EFAULT
`,
		},
		{
			name:          "file exists",
			fd:            sys.FdPreopen,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: ErrnoExist,
			expectedLog: `
==> wasi_snapshot_preview1.path_create_directory(fd=3,path=file)
<== errno=EEXIST
`,
		},
		{
			name:          "dir exists",
			fd:            sys.FdPreopen,
			pathName:      dir,
			path:          0,
			pathLen:       uint32(len(dir)),
			expectedErrno: ErrnoExist,
			expectedLog: `
==> wasi_snapshot_preview1.path_create_directory(fd=3,path=dir)
<== errno=EEXIST
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			mod.Memory().Write(tc.path, []byte(tc.pathName))

			requireErrno(t, tc.expectedErrno, mod, PathCreateDirectoryName, uint64(tc.fd), uint64(tc.path), uint64(tc.pathLen))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_pathFilestatGet(t *testing.T) {
	file, dir, fileInDir := "animals.txt", "sub", "sub/test.txt"

	initialMemoryFile := append([]byte{'?'}, file...)
	initialMemoryDir := append([]byte{'?'}, dir...)
	initialMemoryFileInDir := append([]byte{'?'}, fileInDir...)
	initialMemoryNotExists := []byte{'?', '?'}

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)
	memorySize := mod.Memory().Size()

	fileFD := requireOpenFD(t, mod, file)

	tests := []struct {
		name                        string
		fd, pathLen, resultFilestat uint32
		memory, expectedMemory      []byte
		expectedErrno               Errno
		expectedLog                 string
	}{
		{
			name:           "file under root",
			fd:             sys.FdPreopen,
			memory:         initialMemoryFile,
			pathLen:        uint32(len(file)),
			resultFilestat: uint32(len(file)) + 1,
			expectedMemory: append(
				initialMemoryFile,
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				4, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				30, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // atim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // mtim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // ctim
			),
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=,path=animals.txt)
<== (filestat={filetype=REGULAR_FILE,size=30,mtim=1667482413000000000},errno=ESUCCESS)
`,
		},
		{
			name:           "file under dir",
			fd:             sys.FdPreopen, // root
			memory:         initialMemoryFileInDir,
			pathLen:        uint32(len(fileInDir)),
			resultFilestat: uint32(len(fileInDir)) + 1,
			expectedMemory: append(
				initialMemoryFileInDir,
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				4, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				14, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x0, 0xc2, 0xd3, 0x43, 0x6, 0x36, 0x17, // atim
				0x0, 0x0, 0xc2, 0xd3, 0x43, 0x6, 0x36, 0x17, // mtim
				0x0, 0x0, 0xc2, 0xd3, 0x43, 0x6, 0x36, 0x17, // ctim
			),
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=,path=sub/test.txt)
<== (filestat={filetype=REGULAR_FILE,size=14,mtim=1672531200000000000},errno=ESUCCESS)
`,
		},
		{
			name:           "dir under root",
			fd:             sys.FdPreopen,
			memory:         initialMemoryDir,
			pathLen:        uint32(len(dir)),
			resultFilestat: uint32(len(dir)) + 1,
			expectedMemory: append(
				initialMemoryDir,
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				3, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				0, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // atim
				0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // mtim
				0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // ctim
			),
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=,path=sub)
<== (filestat={filetype=DIRECTORY,size=0,mtim=1640995200000000000},errno=ESUCCESS)
`,
		},
		{
			name:          "unopened FD",
			fd:            math.MaxUint32,
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=-1,flags=,path=)
<== (filestat=,errno=EBADF)
`,
		},
		{
			name:           "FD not a directory",
			fd:             fileFD,
			memory:         initialMemoryFile,
			pathLen:        uint32(len(file)),
			resultFilestat: 2,
			expectedErrno:  ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=4,flags=,path=animals.txt)
<== (filestat=,errno=ENOTDIR)
`,
		},
		{
			name:           "path under root doesn't exist",
			fd:             sys.FdPreopen,
			memory:         initialMemoryNotExists,
			pathLen:        1,
			resultFilestat: 2,
			expectedErrno:  ErrnoNoent,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=,path=?)
<== (filestat=,errno=ENOENT)
`,
		},
		{
			name:          "path is out of memory",
			fd:            sys.FdPreopen,
			memory:        initialMemoryFile,
			pathLen:       memorySize,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=,path=OOM(1,65536))
<== (filestat=,errno=EFAULT)
`,
		},
		{
			name:           "resultFilestat exceeds the maximum valid address by 1",
			fd:             sys.FdPreopen,
			memory:         initialMemoryFile,
			pathLen:        uint32(len(file)),
			resultFilestat: memorySize - 64 + 1,
			expectedErrno:  ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=,path=animals.txt)
<== (filestat=,errno=EFAULT)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, mod, len(tc.expectedMemory))
			mod.Memory().Write(0, tc.memory)

			flags := uint32(0)
			requireErrno(t, tc.expectedErrno, mod, PathFilestatGetName, uint64(tc.fd), uint64(flags), uint64(1), uint64(tc.pathLen), uint64(tc.resultFilestat))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

// Test_pathFilestatSetTimes only tests it is stubbed for GrainLang per #271
func Test_pathFilestatSetTimes(t *testing.T) {
	log := requireErrnoNosys(t, PathFilestatSetTimesName, 0, 0, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.path_filestat_set_times(fd=0,flags=,path=,atim=0,mtim=0,fst_flags=0)
<-- errno=ENOSYS
`, log)
}

// Test_pathLink only tests it is stubbed for GrainLang per #271
func Test_pathLink(t *testing.T) {
	log := requireErrnoNosys(t, PathLinkName, 0, 0, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.path_link(old_fd=0,old_flags=,old_path=,new_fd=0,new_path=)
<-- errno=ENOSYS
`, log)
}

func Test_pathOpen(t *testing.T) {
	dir := t.TempDir() // open before loop to ensure no locking problems.
	writeFS, err := syscallfs.NewDirFS(dir)
	require.NoError(t, err)
	readFS := syscallfs.NewReadFS(writeFS)

	fileName := "file"
	fileContents := []byte("012")
	writeFile(t, dir, fileName, fileContents)

	appendName := "append"
	appendContents := []byte("345")
	writeFile(t, dir, appendName, appendContents)

	truncName := "trunc"
	truncContents := []byte("678")
	writeFile(t, dir, truncName, truncContents)

	dirName := "dir"
	mkdir(t, dir, dirName)

	dirFileName := path.Join(dirName, fileName)
	dirFileContents := []byte("def")
	writeFile(t, dir, dirFileName, dirFileContents)

	expectedOpenedFd := sys.FdPreopen + 1

	tests := []struct {
		name          string
		fs            fs.FS
		path          func(t *testing.T) string
		oflags        uint16
		fdflags       uint16
		expected      func(t *testing.T, fsc *sys.FSContext)
		expectedErrno Errno
		expectedLog   string
	}{
		{
			name: "syscallfs.ReadFS",
			fs:   readFS,
			path: func(*testing.T) string { return fileName },
			expected: func(t *testing.T, fsc *sys.FSContext) {
				requireContents(t, fsc, expectedOpenedFd, fileName, fileContents)
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=file,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
		{
			name: "syscallfs.DirFS",
			fs:   writeFS,
			path: func(*testing.T) string { return fileName },
			expected: func(t *testing.T, fsc *sys.FSContext) {
				requireContents(t, fsc, expectedOpenedFd, fileName, fileContents)
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=file,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
		{
			name:          "syscallfs.ReadFS FD_APPEND",
			fs:            readFS,
			fdflags:       FD_APPEND,
			path:          func(t *testing.T) (file string) { return appendName },
			expectedErrno: ErrnoNosys,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=append,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=APPEND)
<== (opened_fd=,errno=ENOSYS)
`,
		},
		{
			name:    "syscallfs.DirFS FD_APPEND",
			fs:      writeFS,
			path:    func(t *testing.T) (file string) { return appendName },
			fdflags: FD_APPEND,
			expected: func(t *testing.T, fsc *sys.FSContext) {
				contents := []byte("hello")
				_, err := sys.WriterForFile(fsc, expectedOpenedFd).Write(contents)
				require.NoError(t, err)
				require.NoError(t, fsc.CloseFile(expectedOpenedFd))

				// verify the contents were appended
				b := readFile(t, dir, appendName)
				require.Equal(t, append(appendContents, contents...), b)
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=append,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=APPEND)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
		{
			name:          "syscallfs.ReadFS O_CREAT",
			fs:            readFS,
			oflags:        O_CREAT,
			expectedErrno: ErrnoNosys,
			path:          func(*testing.T) string { return "creat" },
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=creat,oflags=CREAT,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=ENOSYS)
`,
		},
		{
			name:   "syscallfs.DirFS O_CREAT",
			fs:     writeFS,
			path:   func(t *testing.T) (file string) { return "creat" },
			oflags: O_CREAT,
			expected: func(t *testing.T, fsc *sys.FSContext) {
				// expect to create a new file
				contents := []byte("hello")
				_, err := sys.WriterForFile(fsc, expectedOpenedFd).Write(contents)
				require.NoError(t, err)
				require.NoError(t, fsc.CloseFile(expectedOpenedFd))

				// verify the contents were written
				b := readFile(t, dir, "creat")
				require.Equal(t, contents, b)
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=creat,oflags=CREAT,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
		{
			name:          "syscallfs.ReadFS O_CREAT O_TRUNC",
			fs:            readFS,
			oflags:        O_CREAT | O_TRUNC,
			expectedErrno: ErrnoNosys,
			path:          func(t *testing.T) (file string) { return path.Join(dirName, "O_CREAT-O_TRUNC") },
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=dir/O_CREAT-O_TRUNC,oflags=CREAT|TRUNC,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=ENOSYS)
`,
		},
		{
			name:   "syscallfs.DirFS O_CREAT O_TRUNC",
			fs:     writeFS,
			path:   func(t *testing.T) (file string) { return path.Join(dirName, "O_CREAT-O_TRUNC") },
			oflags: O_CREAT | O_TRUNC,
			expected: func(t *testing.T, fsc *sys.FSContext) {
				// expect to create a new file
				contents := []byte("hello")
				_, err := sys.WriterForFile(fsc, expectedOpenedFd).Write(contents)
				require.NoError(t, err)
				require.NoError(t, fsc.CloseFile(expectedOpenedFd))

				// verify the contents were written
				b := readFile(t, dir, path.Join(dirName, "O_CREAT-O_TRUNC"))
				require.Equal(t, contents, b)
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=dir/O_CREAT-O_TRUNC,oflags=CREAT|TRUNC,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
		{
			name:   "syscallfs.ReadFS O_DIRECTORY",
			fs:     readFS,
			oflags: O_DIRECTORY,
			path:   func(*testing.T) string { return dirName },
			expected: func(t *testing.T, fsc *sys.FSContext) {
				f, ok := fsc.LookupFile(expectedOpenedFd)
				require.True(t, ok)
				require.True(t, f.IsDir())
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=dir,oflags=DIRECTORY,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
		{
			name:   "syscallfs.DirFS O_DIRECTORY",
			fs:     writeFS,
			path:   func(*testing.T) string { return dirName },
			oflags: O_DIRECTORY,
			expected: func(t *testing.T, fsc *sys.FSContext) {
				f, ok := fsc.LookupFile(expectedOpenedFd)
				require.True(t, ok)
				require.True(t, f.IsDir())
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=dir,oflags=DIRECTORY,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
		{
			name:          "syscallfs.ReadFS O_TRUNC",
			fs:            readFS,
			oflags:        O_TRUNC,
			expectedErrno: ErrnoNosys,
			path:          func(*testing.T) string { return "trunc" },
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=trunc,oflags=TRUNC,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=ENOSYS)
`,
		},
		{
			name:   "syscallfs.DirFS O_TRUNC",
			fs:     writeFS,
			path:   func(t *testing.T) (file string) { return "trunc" },
			oflags: O_TRUNC,
			expected: func(t *testing.T, fsc *sys.FSContext) {
				contents := []byte("hello")
				_, err := sys.WriterForFile(fsc, expectedOpenedFd).Write(contents)
				require.NoError(t, err)
				require.NoError(t, fsc.CloseFile(expectedOpenedFd))

				// verify the contents were truncated
				b := readFile(t, dir, "trunc")
				require.Equal(t, contents, b)
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=trunc,oflags=TRUNC,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(tc.fs))
			defer r.Close(testCtx)
			pathName := tc.path(t)
			mod.Memory().Write(0, []byte(pathName))

			path := uint32(0)
			pathLen := uint32(len(pathName))
			resultOpenedFd := pathLen
			dirfd := sys.FdPreopen

			// TODO: dirflags is a lookupflags and it only has one bit: symlink_follow
			// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#lookupflags
			dirflags := 0

			// rights aren't used
			fsRightsBase, fsRightsInheriting := uint64(0), uint64(0)

			requireErrno(t, tc.expectedErrno, mod, PathOpenName, uint64(dirfd), uint64(dirflags), uint64(path),
				uint64(pathLen), uint64(tc.oflags), fsRightsBase, fsRightsInheriting, uint64(tc.fdflags), uint64(resultOpenedFd))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			if tc.expectedErrno == ErrnoSuccess {
				openedFd, ok := mod.Memory().ReadUint32Le(pathLen)
				require.True(t, ok)
				require.Equal(t, expectedOpenedFd, openedFd)

				tc.expected(t, mod.(*wasm.CallContext).Sys.FS())
			}
		})
	}
}

func requireOpenFD(t *testing.T, mod api.Module, path string) uint32 {
	fsc := mod.(*wasm.CallContext).Sys.FS()

	fd, err := fsc.OpenFile(path, os.O_RDONLY, 0)
	require.NoError(t, err)
	return fd
}

func requireContents(t *testing.T, fsc *sys.FSContext, expectedOpenedFd uint32, fileName string, fileContents []byte) {
	// verify the file was actually opened
	f, ok := fsc.LookupFile(expectedOpenedFd)
	require.True(t, ok)
	require.Equal(t, fileName, f.Name)

	// verify the contents are readable
	b, err := io.ReadAll(f.File)
	require.NoError(t, err)
	require.Equal(t, fileContents, b)
}

func mkdir(t *testing.T, tmpDir, dir string) {
	err := os.Mkdir(path.Join(tmpDir, dir), 0o700)
	require.NoError(t, err)
}

func readFile(t *testing.T, tmpDir, file string) []byte {
	contents, err := os.ReadFile(path.Join(tmpDir, file))
	require.NoError(t, err)
	return contents
}

func writeFile(t *testing.T, tmpDir, file string, contents []byte) {
	err := os.WriteFile(path.Join(tmpDir, file), contents, 0o600)
	require.NoError(t, err)
}

func Test_pathOpen_Errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fs, err := syscallfs.NewDirFS(tmpDir)
	require.NoError(t, err)

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fs))
	defer r.Close(testCtx)

	preopenedFD := sys.FdPreopen

	file := "file"
	err = os.WriteFile(path.Join(tmpDir, file), []byte{}, 0o700)
	require.NoError(t, err)
	fileFD := requireOpenFD(t, mod, file)

	dir := "dir"
	err = os.Mkdir(path.Join(tmpDir, dir), 0o700)
	require.NoError(t, err)

	tests := []struct {
		name, pathName                            string
		fd, path, pathLen, oflags, resultOpenedFd uint32
		expectedErrno                             Errno
		expectedLog                               string
	}{
		{
			name:          "unopened FD",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=42,dirflags=,path=,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=EBADF)
`,
		},
		{
			name:          "FD not a directory",
			fd:            fileFD,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=4,dirflags=,path=file,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=ENOTDIR)
`,
		},
		{
			name:          "out-of-memory reading path",
			fd:            sys.FdPreopen,
			path:          mod.Memory().Size(),
			pathLen:       uint32(len(file)),
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=OOM(65536,4),oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=EFAULT)
`,
		},
		{
			name:          "out-of-memory reading pathLen",
			fd:            sys.FdPreopen,
			path:          0,
			pathLen:       mod.Memory().Size() + 1, // path is in the valid memory range, but pathLen is OOM for path
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=OOM(0,65537),oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=EFAULT)
`,
		},
		{
			name:          "no such file exists",
			fd:            sys.FdPreopen,
			pathName:      dir,
			path:          0,
			pathLen:       uint32(len(dir)) - 1,
			expectedErrno: ErrnoNoent,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=di,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=ENOENT)
`,
		},
		{
			name:           "out-of-memory writing resultOpenedFd",
			fd:             preopenedFD,
			pathName:       dir,
			path:           0,
			pathLen:        uint32(len(dir)),
			resultOpenedFd: mod.Memory().Size(), // path and pathLen correctly point to the right path, but where to write the opened FD is outside memory.
			expectedErrno:  ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=dir,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=EFAULT)
`,
		},
		{
			name:          "O_DIRECTORY, but not a directory",
			oflags:        uint32(O_DIRECTORY),
			fd:            sys.FdPreopen,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=file,oflags=DIRECTORY,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=ENOTDIR)
`,
		},
		{
			name:          "oflags=directory and create invalid",
			oflags:        uint32(O_DIRECTORY | O_CREAT),
			fd:            sys.FdPreopen,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=file,oflags=CREAT|DIRECTORY,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=EINVAL)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			mod.Memory().Write(tc.path, []byte(tc.pathName))

			requireErrno(t, tc.expectedErrno, mod, PathOpenName, uint64(tc.fd), uint64(0), uint64(tc.path),
				uint64(tc.pathLen), uint64(tc.oflags), 0, 0, 0, uint64(tc.resultOpenedFd))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

// Test_pathReadlink only tests it is stubbed for GrainLang per #271
func Test_pathReadlink(t *testing.T) {
	log := requireErrnoNosys(t, PathReadlinkName, 0, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.path_readlink(fd=0,path=,buf=0,buf_len=0,result.bufused=0)
<-- errno=ENOSYS
`, log)
}

func Test_pathRemoveDirectory(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fs, err := syscallfs.NewDirFS(tmpDir)
	require.NoError(t, err)

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fs))
	defer r.Close(testCtx)

	// set up the initial memory to include the path name starting at an offset.
	pathName := "wazero"
	realPath := path.Join(tmpDir, pathName)
	ok := mod.Memory().Write(0, append([]byte{'?'}, pathName...))
	require.True(t, ok)

	// create the directory
	err = os.Mkdir(realPath, 0o700)
	require.NoError(t, err)

	dirFD := sys.FdPreopen
	name := 1
	nameLen := len(pathName)

	requireErrno(t, ErrnoSuccess, mod, PathRemoveDirectoryName, uint64(dirFD), uint64(name), uint64(nameLen))
	require.Equal(t, `
==> wasi_snapshot_preview1.path_remove_directory(fd=3,path=wazero)
<== errno=ESUCCESS
`, "\n"+log.String())

	// ensure the directory was removed
	_, err = os.Stat(realPath)
	require.Error(t, err)
}

func Test_pathRemoveDirectory_Errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fs, err := syscallfs.NewDirFS(tmpDir)
	require.NoError(t, err)

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fs))
	defer r.Close(testCtx)

	file := "file"
	err = os.WriteFile(path.Join(tmpDir, file), []byte{}, 0o700)
	require.NoError(t, err)
	fileFD := requireOpenFD(t, mod, file)

	dirNotEmpty := "notempty"
	err = os.Mkdir(path.Join(tmpDir, dirNotEmpty), 0o700)
	require.NoError(t, err)

	dir := "dir"
	err = os.Mkdir(path.Join(tmpDir, dirNotEmpty, dir), 0o700)
	require.NoError(t, err)

	tests := []struct {
		name, pathName    string
		fd, path, pathLen uint32
		expectedErrno     Errno
		expectedLog       string
	}{
		{
			name:          "unopened FD",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_remove_directory(fd=42,path=)
<== errno=EBADF
`,
		},
		{
			name:          "FD not a directory",
			fd:            fileFD,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_remove_directory(fd=4,path=file)
<== errno=ENOTDIR
`,
		},
		{
			name:          "out-of-memory reading path",
			fd:            sys.FdPreopen,
			path:          mod.Memory().Size(),
			pathLen:       1,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_remove_directory(fd=3,path=OOM(65536,1))
<== errno=EFAULT
`,
		},
		{
			name:          "out-of-memory reading pathLen",
			fd:            sys.FdPreopen,
			path:          0,
			pathLen:       mod.Memory().Size() + 1, // path is in the valid memory range, but pathLen is OOM for path
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_remove_directory(fd=3,path=OOM(0,65537))
<== errno=EFAULT
`,
		},
		{
			name:          "no such file exists",
			fd:            sys.FdPreopen,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file) - 1),
			expectedErrno: ErrnoNoent,
			expectedLog: `
==> wasi_snapshot_preview1.path_remove_directory(fd=3,path=fil)
<== errno=ENOENT
`,
		},
		{
			name:          "file not dir",
			fd:            sys.FdPreopen,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: errNotDir(),
			expectedLog: fmt.Sprintf(`
==> wasi_snapshot_preview1.path_remove_directory(fd=3,path=file)
<== errno=%s
`, ErrnoName(errNotDir())),
		},
		{
			name:          "dir not empty",
			fd:            sys.FdPreopen,
			pathName:      dirNotEmpty,
			path:          0,
			pathLen:       uint32(len(dirNotEmpty)),
			expectedErrno: ErrnoNotempty,
			expectedLog: `
==> wasi_snapshot_preview1.path_remove_directory(fd=3,path=notempty)
<== errno=ENOTEMPTY
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			mod.Memory().Write(tc.path, []byte(tc.pathName))

			requireErrno(t, tc.expectedErrno, mod, PathRemoveDirectoryName, uint64(tc.fd), uint64(tc.path), uint64(tc.pathLen))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func errNotDir() Errno {
	if runtime.GOOS == "windows" {
		// As of Go 1.19, Windows maps syscall.ENOTDIR to syscall.ENOENT
		return ErrnoNoent
	}
	return ErrnoNotdir
}

// Test_pathSymlink only tests it is stubbed for GrainLang per #271
func Test_pathSymlink(t *testing.T) {
	log := requireErrnoNosys(t, PathSymlinkName, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.path_symlink(old_path=,fd=0,new_path=)
<-- errno=ENOSYS
`, log)
}

func Test_pathRename(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fs, err := syscallfs.NewDirFS(tmpDir)
	require.NoError(t, err)

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fs))
	defer r.Close(testCtx)

	// set up the initial memory to include the old path name starting at an offset.
	oldDirFD := sys.FdPreopen
	oldPathName := "wazero"
	realOldPath := path.Join(tmpDir, oldPathName)
	oldPath := uint32(0)
	oldPathLen := len(oldPathName)
	ok := mod.Memory().Write(oldPath, []byte(oldPathName))
	require.True(t, ok)

	// create the file
	err = os.WriteFile(realOldPath, []byte{}, 0o600)
	require.NoError(t, err)

	newDirFD := sys.FdPreopen
	newPathName := "wahzero"
	realNewPath := path.Join(tmpDir, newPathName)
	newPath := uint32(16)
	newPathLen := len(newPathName)
	ok = mod.Memory().Write(newPath, []byte(newPathName))
	require.True(t, ok)

	requireErrno(t, ErrnoSuccess, mod, PathRenameName,
		uint64(oldDirFD), uint64(oldPath), uint64(oldPathLen),
		uint64(newDirFD), uint64(newPath), uint64(newPathLen))
	require.Equal(t, `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=wazero,new_fd=3,new_path=wahzero)
<== errno=ESUCCESS
`, "\n"+log.String())

	// ensure the file was renamed
	_, err = os.Stat(realOldPath)
	require.Error(t, err)
	_, err = os.Stat(realNewPath)
	require.NoError(t, err)
}

func Test_pathRename_Errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fs, err := syscallfs.NewDirFS(tmpDir)
	require.NoError(t, err)

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fs))
	defer r.Close(testCtx)

	file := "file"
	err = os.WriteFile(path.Join(tmpDir, file), []byte{}, 0o700)
	require.NoError(t, err)

	// We have to test FD validation with a path not under test. Otherwise,
	// Windows may fail for the wrong reason, like:
	//	The process cannot access the file because it is being used by another process.
	file1 := "file1"
	err = os.WriteFile(path.Join(tmpDir, file1), []byte{}, 0o700)
	require.NoError(t, err)
	fileFD := requireOpenFD(t, mod, file1)

	dirNotEmpty := "notempty"
	err = os.Mkdir(path.Join(tmpDir, dirNotEmpty), 0o700)
	require.NoError(t, err)

	dir := path.Join(dirNotEmpty, "dir")
	err = os.Mkdir(path.Join(tmpDir, dir), 0o700)
	require.NoError(t, err)

	tests := []struct {
		name, oldPathName, newPathName string
		oldFd, oldPath, oldPathLen     uint32
		newFd, newPath, newPathLen     uint32
		expectedErrno                  Errno
		expectedLog                    string
	}{
		{
			name:          "unopened old fd",
			oldFd:         42, // arbitrary invalid fd
			newFd:         sys.FdPreopen,
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=42,old_path=,new_fd=3,new_path=)
<== errno=EBADF
`,
		},
		{
			name:          "old FD not a directory",
			oldFd:         fileFD,
			newFd:         sys.FdPreopen,
			expectedErrno: ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=4,old_path=,new_fd=3,new_path=)
<== errno=ENOTDIR
`,
		},
		{
			name:          "unopened new fd",
			oldFd:         sys.FdPreopen,
			newFd:         42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=,new_fd=42,new_path=)
<== errno=EBADF
`,
		},
		{
			name:          "new FD not a directory",
			oldFd:         sys.FdPreopen,
			newFd:         fileFD,
			expectedErrno: ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=,new_fd=4,new_path=)
<== errno=ENOTDIR
`,
		},
		{
			name:          "out-of-memory reading old path",
			oldFd:         sys.FdPreopen,
			newFd:         sys.FdPreopen,
			oldPath:       mod.Memory().Size(),
			oldPathLen:    1,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=OOM(65536,1),new_fd=3,new_path=)
<== errno=EFAULT
`,
		},
		{
			name:          "out-of-memory reading new path",
			oldFd:         sys.FdPreopen,
			newFd:         sys.FdPreopen,
			oldPath:       0,
			oldPathName:   "a",
			oldPathLen:    1,
			newPath:       mod.Memory().Size(),
			newPathLen:    1,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=a,new_fd=3,new_path=OOM(65536,1))
<== errno=EFAULT
`,
		},
		{
			name:          "out-of-memory reading old pathLen",
			oldFd:         sys.FdPreopen,
			newFd:         sys.FdPreopen,
			oldPath:       0,
			oldPathLen:    mod.Memory().Size() + 1, // path is in the valid memory range, but pathLen is OOM for path
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=OOM(0,65537),new_fd=3,new_path=)
<== errno=EFAULT
`,
		},
		{
			name:          "out-of-memory reading new pathLen",
			oldFd:         sys.FdPreopen,
			newFd:         sys.FdPreopen,
			oldPathName:   file,
			oldPathLen:    uint32(len(file)),
			newPath:       0,
			newPathLen:    mod.Memory().Size() + 1, // path is in the valid memory range, but pathLen is OOM for path
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=file,new_fd=3,new_path=OOM(0,65537))
<== errno=EFAULT
`,
		},
		{
			name:          "no such file exists",
			oldFd:         sys.FdPreopen,
			newFd:         sys.FdPreopen,
			oldPathName:   file,
			oldPathLen:    uint32(len(file)) - 1,
			newPath:       16,
			newPathName:   file,
			newPathLen:    uint32(len(file)),
			expectedErrno: ErrnoNoent,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=fil,new_fd=3,new_path=file)
<== errno=ENOENT
`,
		},
		{
			name:          "dir not file",
			oldFd:         sys.FdPreopen,
			newFd:         sys.FdPreopen,
			oldPathName:   file,
			oldPathLen:    uint32(len(file)),
			newPath:       16,
			newPathName:   dir,
			newPathLen:    uint32(len(dir)),
			expectedErrno: ErrnoIsdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=file,new_fd=3,new_path=notempty/dir)
<== errno=EISDIR
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			mod.Memory().Write(tc.oldPath, []byte(tc.oldPathName))
			mod.Memory().Write(tc.newPath, []byte(tc.newPathName))

			requireErrno(t, tc.expectedErrno, mod, PathRenameName,
				uint64(tc.oldFd), uint64(tc.oldPath), uint64(tc.oldPathLen),
				uint64(tc.newFd), uint64(tc.newPath), uint64(tc.newPathLen))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_pathUnlinkFile(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fs, err := syscallfs.NewDirFS(tmpDir)
	require.NoError(t, err)

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fs))
	defer r.Close(testCtx)

	// set up the initial memory to include the path name starting at an offset.
	pathName := "wazero"
	realPath := path.Join(tmpDir, pathName)
	ok := mod.Memory().Write(0, append([]byte{'?'}, pathName...))
	require.True(t, ok)

	// create the file
	err = os.WriteFile(realPath, []byte{}, 0o600)
	require.NoError(t, err)

	dirFD := sys.FdPreopen
	name := 1
	nameLen := len(pathName)

	requireErrno(t, ErrnoSuccess, mod, PathUnlinkFileName, uint64(dirFD), uint64(name), uint64(nameLen))
	require.Equal(t, `
==> wasi_snapshot_preview1.path_unlink_file(fd=3,path=wazero)
<== errno=ESUCCESS
`, "\n"+log.String())

	// ensure the file was removed
	_, err = os.Stat(realPath)
	require.Error(t, err)
}

func Test_pathUnlinkFile_Errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fs, err := syscallfs.NewDirFS(tmpDir)
	require.NoError(t, err)

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fs))
	defer r.Close(testCtx)

	file := "file"
	err = os.WriteFile(path.Join(tmpDir, file), []byte{}, 0o700)
	require.NoError(t, err)
	fileFD := requireOpenFD(t, mod, file)

	dir := "dir"
	err = os.Mkdir(path.Join(tmpDir, dir), 0o700)
	require.NoError(t, err)

	tests := []struct {
		name, pathName    string
		fd, path, pathLen uint32
		expectedErrno     Errno
		expectedLog       string
	}{
		{
			name:          "unopened FD",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_unlink_file(fd=42,path=)
<== errno=EBADF
`,
		},
		{
			name:          "FD not a directory",
			fd:            fileFD,
			expectedErrno: ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_unlink_file(fd=4,path=)
<== errno=ENOTDIR
`,
		},
		{
			name:          "out-of-memory reading path",
			fd:            sys.FdPreopen,
			path:          mod.Memory().Size(),
			pathLen:       1,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_unlink_file(fd=3,path=OOM(65536,1))
<== errno=EFAULT
`,
		},
		{
			name:          "out-of-memory reading pathLen",
			fd:            sys.FdPreopen,
			path:          0,
			pathLen:       mod.Memory().Size() + 1, // path is in the valid memory range, but pathLen is OOM for path
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_unlink_file(fd=3,path=OOM(0,65537))
<== errno=EFAULT
`,
		},
		{
			name:          "no such file exists",
			fd:            sys.FdPreopen,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file) - 1),
			expectedErrno: ErrnoNoent,
			expectedLog: `
==> wasi_snapshot_preview1.path_unlink_file(fd=3,path=fil)
<== errno=ENOENT
`,
		},
		{
			name:          "dir not file",
			fd:            sys.FdPreopen,
			pathName:      dir,
			path:          0,
			pathLen:       uint32(len(dir)),
			expectedErrno: ErrnoIsdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_unlink_file(fd=3,path=dir)
<== errno=EISDIR
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			mod.Memory().Write(tc.path, []byte(tc.pathName))

			requireErrno(t, tc.expectedErrno, mod, PathUnlinkFileName, uint64(tc.fd), uint64(tc.path), uint64(tc.pathLen))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func requireOpenFile(t *testing.T, tmpDir string, pathName string, data []byte, readOnly bool) (api.Module, uint32, *bytes.Buffer, api.Closer) {
	oflags := os.O_RDWR

	realPath := path.Join(tmpDir, pathName)
	if data == nil {
		oflags = os.O_RDONLY
		require.NoError(t, os.Mkdir(realPath, 0o700))
	} else {
		require.NoError(t, os.WriteFile(realPath, data, 0o600))
	}

	writeFS, err := syscallfs.NewDirFS(tmpDir)
	require.NoError(t, err)

	testFS := writeFS
	if readOnly {
		oflags = os.O_RDONLY
		testFS = syscallfs.NewReadFS(testFS)
	}

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(testFS))
	fsc := mod.(*wasm.CallContext).Sys.FS()

	fd, err := fsc.OpenFile(pathName, oflags, 0)
	require.NoError(t, err)

	return mod, fd, log, r
}
