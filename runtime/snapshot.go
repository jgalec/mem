package runtime

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

func (r *Runtime) StoreSnapshot(projectId string, context map[string]interface{}) (string, error) {
	data, err := json.Marshal(context)
	if err != nil {
		return "", fmt.Errorf("snapshot marshal: %w", err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))[:12]
	key := projectId + ":startup_context_" + hash

	r.snapshotMu.Lock()
	r.snapshots[key] = data
	r.snapshotMu.Unlock()

	return "startup_context_" + hash, nil
}

func (r *Runtime) GetSnapshot(projectId, key string) (map[string]interface{}, bool) {
	fullKey := projectId + ":" + key
	r.snapshotMu.RLock()
	data, ok := r.snapshots[fullKey]
	r.snapshotMu.RUnlock()
	if !ok {
		return nil, false
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, false
	}
	return result, true
}

func (r *Runtime) InvalidSnapshot(projectId, key string) {
	r.snapshotMu.Lock()
	delete(r.snapshots, projectId+":"+key)
	r.snapshotMu.Unlock()
}

func (r *Runtime) InvalidProjectSnapshots(projectId string) {
	prefix := projectId + ":"
	r.snapshotMu.Lock()
	for k := range r.snapshots {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(r.snapshots, k)
		}
	}
	r.snapshotMu.Unlock()
}
