package ops

import (
	"encoding/json"
	"fmt"
	"strings"

	fstype "github.com/yourname/fs-engine/internal/fs"
)

// xattrStore is a simple in-memory store for extended attributes.
// In a real implementation this would be stored on disk.
// Key format: "inum:name"
var globalXattrStore = newXattrStore()

type xattrStore struct {
	data map[string][]byte
}

func newXattrStore() *xattrStore {
	return &xattrStore{data: make(map[string][]byte)}
}

func (s *xattrStore) key(inum uint32, name string) string {
	return fmt.Sprintf("%d:%s", inum, name)
}

func (s *xattrStore) get(inum uint32, name string) ([]byte, bool) {
	v, ok := s.data[s.key(inum, name)]
	return v, ok
}

func (s *xattrStore) set(inum uint32, name string, value []byte) {
	s.data[s.key(inum, name)] = value
}

func (s *xattrStore) delete(inum uint32, name string) {
	delete(s.data, s.key(inum, name))
}

func (s *xattrStore) list(inum uint32) []string {
	prefix := fmt.Sprintf("%d:", inum)
	var names []string
	for k := range s.data {
		if strings.HasPrefix(k, prefix) {
			names = append(names, strings.TrimPrefix(k, prefix))
		}
	}
	return names
}

// GetXAttr returns the extended attribute value for the file at path.
func GetXAttr(fs *fstype.Filesystem, cred Credential, path, name string) ([]byte, error) {
	inum, err := Resolve(fs, cred, path)
	if err != nil {
		return nil, err
	}

	di, err := fs.ReadInode(inum)
	if err != nil {
		return nil, err
	}

	if err := CheckPermission(di, cred, true, false, false); err != nil {
		return nil, err
	}

	val, ok := globalXattrStore.get(inum, name)
	if !ok {
		return nil, fmt.Errorf("xattr: %s: attribute not found", name)
	}
	return val, nil
}

// SetXAttr sets an extended attribute on the file at path.
func SetXAttr(fs *fstype.Filesystem, cred Credential, path, name string, value []byte, flags int) error {
	inum, err := Resolve(fs, cred, path)
	if err != nil {
		return err
	}

	di, err := fs.ReadInode(inum)
	if err != nil {
		return err
	}

	if err := CheckPermission(di, cred, false, true, false); err != nil {
		return err
	}

	if len(name) == 0 || len(name) > 255 {
		return fstype.ErrInvalidArg
	}
	if len(value) > 65536 {
		return fstype.ErrNoSpace
	}

	cp := make([]byte, len(value))
	copy(cp, value)
	globalXattrStore.set(inum, name, cp)
	return nil
}

// ListXAttr returns all extended attribute names for the file at path.
func ListXAttr(fs *fstype.Filesystem, cred Credential, path string) ([]string, error) {
	inum, err := Resolve(fs, cred, path)
	if err != nil {
		return nil, err
	}

	di, err := fs.ReadInode(inum)
	if err != nil {
		return nil, err
	}

	if err := CheckPermission(di, cred, true, false, false); err != nil {
		return nil, err
	}

	return globalXattrStore.list(inum), nil
}

// RemoveXAttr removes an extended attribute from the file at path.
func RemoveXAttr(fs *fstype.Filesystem, cred Credential, path, name string) error {
	inum, err := Resolve(fs, cred, path)
	if err != nil {
		return err
	}

	di, err := fs.ReadInode(inum)
	if err != nil {
		return err
	}

	if err := CheckPermission(di, cred, false, true, false); err != nil {
		return err
	}

	if _, ok := globalXattrStore.get(inum, name); !ok {
		return fmt.Errorf("xattr: %s: attribute not found", name)
	}

	globalXattrStore.delete(inum, name)
	return nil
}

// XAttrJSON returns all xattrs for an inode as a JSON-encoded map.
func XAttrJSON(fs *fstype.Filesystem, cred Credential, path string) ([]byte, error) {
	names, err := ListXAttr(fs, cred, path)
	if err != nil {
		return nil, err
	}

	inum, err := Resolve(fs, cred, path)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string, len(names))
	for _, name := range names {
		val, ok := globalXattrStore.get(inum, name)
		if ok {
			result[name] = string(val)
		}
	}

	return json.Marshal(result)
}
