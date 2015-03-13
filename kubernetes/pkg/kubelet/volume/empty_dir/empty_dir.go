/*
Copyright 2014 Google Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package empty_dir

import (
	"fmt"
	"os"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/kubelet/volume"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/types"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util/mount"
)

// This is the primary entrypoint for volume plugins.
func ProbeVolumePlugins() []volume.Plugin {
	return []volume.Plugin{&emptyDirPlugin{nil, false}, &emptyDirPlugin{nil, true}}
}

type emptyDirPlugin struct {
	host       volume.Host
	legacyMode bool // if set, plugin answers to the legacy name
}

var _ volume.Plugin = &emptyDirPlugin{}

const (
	emptyDirPluginName       = "kubernetes.io/empty-dir"
	emptyDirPluginLegacyName = "empty"
)

func (plugin *emptyDirPlugin) Init(host volume.Host) {
	plugin.host = host
}

func (plugin *emptyDirPlugin) Name() string {
	if plugin.legacyMode {
		return emptyDirPluginLegacyName
	}
	return emptyDirPluginName
}

func (plugin *emptyDirPlugin) CanSupport(spec *api.Volume) bool {
	if plugin.legacyMode {
		// Legacy mode instances can be cleaned up but not created anew.
		return false
	}

	if util.AllPtrFieldsNil(&spec.VolumeSource) {
		return true
	}
	if spec.EmptyDir != nil {
		return true
	}
	return false
}

func (plugin *emptyDirPlugin) NewBuilder(spec *api.Volume, podRef *api.ObjectReference) (volume.Builder, error) {
	// Inject real implementations here, test through the internal function.
	return plugin.newBuilderInternal(spec, podRef, mount.New(), &realMediumer{})
}

func (plugin *emptyDirPlugin) newBuilderInternal(spec *api.Volume, podRef *api.ObjectReference, mounter mount.Interface, mediumer mediumer) (volume.Builder, error) {
	if plugin.legacyMode {
		// Legacy mode instances can be cleaned up but not created anew.
		return nil, fmt.Errorf("legacy mode: can not create new instances")
	}
	medium := api.StorageTypeDefault
	if spec.EmptyDir != nil { // Support a non-specified source as EmptyDir.
		medium = spec.EmptyDir.Medium
	}
	return &emptyDir{
		podUID:     podRef.UID,
		volName:    spec.Name,
		medium:     medium,
		mediumer:   mediumer,
		mounter:    mounter,
		plugin:     plugin,
		legacyMode: false,
	}, nil
}

func (plugin *emptyDirPlugin) NewCleaner(volName string, podUID types.UID) (volume.Cleaner, error) {
	// Inject real implementations here, test through the internal function.
	return plugin.newCleanerInternal(volName, podUID, mount.New(), &realMediumer{})
}

func (plugin *emptyDirPlugin) newCleanerInternal(volName string, podUID types.UID, mounter mount.Interface, mediumer mediumer) (volume.Cleaner, error) {
	legacy := false
	if plugin.legacyMode {
		legacy = true
	}
	ed := &emptyDir{
		podUID:     podUID,
		volName:    volName,
		medium:     api.StorageTypeDefault, // might be changed later
		mounter:    mounter,
		mediumer:   mediumer,
		plugin:     plugin,
		legacyMode: legacy,
	}
	// Figure out the medium.
	if medium, err := mediumer.GetMedium(ed.GetPath()); err != nil {
		return nil, err
	} else {
		switch medium {
		case mediumMemory:
			ed.medium = api.StorageTypeMemory
		default:
			// assume StorageTypeDefault
		}
	}
	return ed, nil
}

// mediumer abstracts how to find what storageMedium a path is backed by.
type mediumer interface {
	GetMedium(path string) (storageMedium, error)
}

type storageMedium int

const (
	mediumUnknown storageMedium = 0 // assume anything we don't explicitly handle is this
	mediumMemory  storageMedium = 1 // memory (e.g. tmpfs on linux)
)

// EmptyDir volumes are temporary directories exposed to the pod.
// These do not persist beyond the lifetime of a pod.
type emptyDir struct {
	podUID     types.UID
	volName    string
	medium     api.StorageType
	mounter    mount.Interface
	mediumer   mediumer
	plugin     *emptyDirPlugin
	legacyMode bool
}

// SetUp creates new directory.
func (ed *emptyDir) SetUp() error {
	if ed.legacyMode {
		return fmt.Errorf("legacy mode: can not create new instances")
	}
	switch ed.medium {
	case api.StorageTypeDefault:
		return ed.setupDefault()
	case api.StorageTypeMemory:
		return ed.setupTmpfs()
	default:
		return fmt.Errorf("unknown storage medium %q", ed.medium)
	}
}

func (ed *emptyDir) setupDefault() error {
	return os.MkdirAll(ed.GetPath(), 0750)
}

func (ed *emptyDir) setupTmpfs() error {
	if ed.mounter == nil {
		return fmt.Errorf("memory storage requested, but mounter is nil")
	}
	if err := os.MkdirAll(ed.GetPath(), 0750); err != nil {
		return err
	}
	// Make SetUp idempotent.
	if medium, err := ed.mediumer.GetMedium(ed.GetPath()); err != nil {
		return err
	} else if medium == mediumMemory {
		return nil // current state is what we expect
	}
	return ed.mounter.Mount("tmpfs", ed.GetPath(), "tmpfs", 0, "")
}

func (ed *emptyDir) GetPath() string {
	name := emptyDirPluginName
	if ed.legacyMode {
		name = emptyDirPluginLegacyName
	}
	return ed.plugin.host.GetPodVolumeDir(ed.podUID, volume.EscapePluginName(name), ed.volName)
}

// TearDown simply discards everything in the directory.
func (ed *emptyDir) TearDown() error {
	switch ed.medium {
	case api.StorageTypeDefault:
		return ed.teardownDefault()
	case api.StorageTypeMemory:
		return ed.teardownTmpfs()
	default:
		return fmt.Errorf("unknown storage medium %q", ed.medium)
	}
}

func (ed *emptyDir) teardownDefault() error {
	tmpDir, err := volume.RenameDirectory(ed.GetPath(), ed.volName+".deleting~")
	if err != nil {
		return err
	}
	err = os.RemoveAll(tmpDir)
	if err != nil {
		return err
	}
	return nil
}

func (ed *emptyDir) teardownTmpfs() error {
	if ed.mounter == nil {
		return fmt.Errorf("memory storage requested, but mounter is nil")
	}
	if err := ed.mounter.Unmount(ed.GetPath(), 0); err != nil {
		return err
	}
	if err := os.RemoveAll(ed.GetPath()); err != nil {
		return err
	}
	return nil
}
