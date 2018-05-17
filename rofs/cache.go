package rofs

import (
	"fmt"
	"os"
	"path"
	"syscall"
	"time"

	"github.com/zero-os/0-fs/meta"
)

func (fs *filesystem) path(hash string) string {
	return path.Join(fs.cache, hash)
}

func (fs *filesystem) checkAndGet(m meta.Meta) (*os.File, error) {
	//atomic check and download a file
	name := fs.path(m.ID())
	f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, os.ModePerm&os.FileMode(0755))
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return nil, err
	}

	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	fstat, err := f.Stat()

	if err != nil {
		return nil, err
	}

	info := m.Info()
	if fstat.Size() == int64(info.Size) {
		return f, nil
	}

	// concurrently ask peer and the hub about the file

	cP2P := fs.p2pNode.CheckAndGet(m.ID())

	cHub := make(chan error)
	go func(f *os.File) {
		err := fs.download(f, m)
		cHub <- err
		close(cHub)
	}(f)

	err = nil
	for cP2P != nil || cHub != nil {
		select {
		case data, ok := <-cP2P:
			log.Infof("a peer as accepted to share the file with us\n")
			if !ok {
				cP2P = nil // don't try to received on this channel anymore
			}

			_, err = f.Write(data)
			if err != nil {
				log.Errorf("fail to download from a peer :%v", err)
				continue
			}

			log.Infof("we fetched the file from a peer")
			// make condition of the loop false
			cHub = nil

		case err = <-cHub:
			if err != nil {
				log.Errorf("fail to download from the hub :%v", err)
				cHub = nil
				continue
			}

			log.Infof("we fetched the file from the hub")
			// make condition of the loop false
			cP2P = nil
			cHub = nil

		case <-time.After(time.Second * 10):
			// make condition of the loop false
			cP2P = nil
			cHub = nil
			err = fmt.Errorf("timeout for fetching file %s", m.Name())
		}
	}

	if err != nil {
		log.Errorf("error downloading file: %v", err)
		f.Close()
		os.Remove(name)
		return nil, err
	}

	f.Seek(0, os.SEEK_SET)
	return f, nil
}

// download file from storage
func (fs *filesystem) download(file *os.File, m meta.Meta) error {
	downloader := Downloader{
		Storage:   fs.storage,
		BlockSize: m.Info().FileBlockSize,
		Blocks:    m.Blocks(),
	}

	return downloader.Download(file)
}
