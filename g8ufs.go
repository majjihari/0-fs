package g8ufs

import (
	"fmt"
	"github.com/g8os/g8ufs/meta"
	"github.com/g8os/g8ufs/rofs"
	"github.com/g8os/g8ufs/storage"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
	"github.com/op/go-logging"
	"os"
	"os/exec"
	"path"
	"syscall"
	"time"
)

var (
	log = logging.MustGetLogger("g8ufs")
)

type Starter interface {
	Start() error
	Wait() error
}

type Exec func(name string, arg ...string) Starter

type Options struct {
	//Backend (required) working directory where the filesystem keeps it's cache and others
	//will be created if doesn't exist
	Backend string
	//Cache location where downloaded files are gonna be kept (optional). If not provided
	//a cache directly will be created under the backend.
	Cache string
	//Mount (required) is the mount point
	Target string
	//MetaStore (optional) will use meta.NewMemoryMeta if not provided
	MetaStore meta.MetaStore
	//Storage (required) storage to download files from
	Storage storage.Storage
	//Reset if set, will wipe up the backend clean before mounting.
	Reset bool
}

type G8ufs struct {
	target string
	fuse   string
	server *fuse.Server
}

//Mount mounts fuse with given options, it blocks forever until unmount is called on the given mount point
func Mount(opt *Options) (*G8ufs, error) {
	if opt.MetaStore == nil {
		return nil, fmt.Errorf("missing meta store")
	}
	backend := opt.Backend
	ro := path.Join(backend, "ro") //ro lower layer provided by fuse
	rw := path.Join(backend, "rw") //rw upper layer on filyestem
	wd := path.Join(backend, "wd") //wd workdir used by overlayfs
	toSetup := []string{ro, rw, wd}
	ca := path.Join(backend, "ca") //ca cache for downloaded files used by fuse
	if opt.Cache != "" {
		ca = opt.Cache
		os.MkdirAll(ca, 0755)
	} else {
		toSetup = append(toSetup, ca)
	}

	for _, name := range toSetup {
		if opt.Reset {
			os.RemoveAll(name)
		}
		os.MkdirAll(name, 0755)
	}

	fs := rofs.New(opt.Storage, opt.MetaStore, ca)

	server, err := fuse.NewServer(
		nodefs.NewFileSystemConnector(
			pathfs.NewPathNodeFs(fs, nil).Root(),
			nil,
		).RawFS(), ro, &fuse.MountOptions{
			AllowOther: true,
			Options:    []string{"ro"},
		})

	if err != nil {
		return nil, err
	}

	go server.Serve()
	log.Debugf("Waiting for fuse mount")
	server.WaitMount()

	log.Debugf("Fuse mount is complete")

	err = syscall.Mount("overlay",
		opt.Target,
		"overlay",
		syscall.MS_NOATIME,
		fmt.Sprintf(
			"lowerdir=%s,upperdir=%s,workdir=%s",
			ro, rw, wd,
		),
	)

	if err != nil {
		server.Unmount()
		return nil, err
	}

	success := false
	for i := 0; i < 5; i++ {
		//wait for mount point
		chk := exec.Command("mountpoint", "-q", opt.Target)
		if err := chk.Run(); err != nil {
			log.Debugf("mount point still not ready: %s", err)
			time.Sleep(time.Second)
			continue
		}
		success = true
		break
	}

	if !success {
		server.Unmount()
		return nil, fmt.Errorf("failed to start mount")
	}

	return &G8ufs{
		target: opt.Target,
		server: server,
		fuse:   ro,
	}, nil
}

//Wait filesystem until it's unmounted.
func (fs *G8ufs) Wait() error {
	defer func() {
		fs.umountFuse()
	}()
	//Wait for filesystem to be unmounted.
	fd, err := syscall.InotifyInit()
	if err != nil {
		return err
	}

	defer syscall.Close(fd)

	if _, err := syscall.InotifyAddWatch(fd, fs.target, syscall.IN_UNMOUNT); err != nil {
		return err
	}

	buff := make([]byte, syscall.SizeofInotifyEvent)
	if _, err := syscall.Read(fd, buff); err != nil {
		return err
	}

	return nil
}

type errors []interface{}

func (e errors) Error() string {
	return fmt.Sprint(e...)
}

func (fs *G8ufs) umountFuse() error {
	if err := syscall.Unmount(fs.fuse, syscall.MNT_FORCE|syscall.MNT_DETACH); err != nil {
		return err
	}

	return nil
}

func (fs *G8ufs) Unmount() error {
	var errs errors

	if err := syscall.Unmount(fs.target, syscall.MNT_FORCE|syscall.MNT_DETACH); err != nil {
		errs = append(errs, err)
	}

	if err := fs.umountFuse(); err != nil {
		errs = append(errs, err)
	}

	return errs
}
