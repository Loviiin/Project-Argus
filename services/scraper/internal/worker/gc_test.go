package worker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProfileSweeperLogic(t *testing.T) {
	// 1. Setup mock environment
	tempDir := t.TempDir()

	activeProfile := filepath.Join(tempDir, "argus_profile_active123")
	orphanProfile := filepath.Join(tempDir, "argus_profile_orphan456")
	unrelatedFolder := filepath.Join(tempDir, "some_other_folder")

	if err := os.Mkdir(activeProfile, 0755); err != nil {
		t.Fatalf("Erro criando active profile mock: %v", err)
	}
	if err := os.Mkdir(orphanProfile, 0755); err != nil {
		t.Fatalf("Erro criando orphan profile mock: %v", err)
	}
	if err := os.Mkdir(unrelatedFolder, 0755); err != nil {
		t.Fatalf("Erro criando unrelated folder mock: %v", err)
	}

	// Mock timestamps
	now := time.Now()

	// Active profile is recent (10 mins old)
	if err := os.Chtimes(activeProfile, now.Add(-10*time.Minute), now.Add(-10*time.Minute)); err != nil {
		t.Fatalf("Erro mockando tempo do active profile: %v", err)
	}
	// Unrelated profile is old but shouldn't be touched (prefix didn't match)
	if err := os.Chtimes(unrelatedFolder, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("Erro mockando tempo da unrelated folder: %v", err)
	}
	// Orphan profile is old (> 90 mins)
	if err := os.Chtimes(orphanProfile, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("Erro mockando tempo do orphan profile: %v", err)
	}

	// 2. Perform act
	ttl := 90 * time.Minute
	sweepOrphanProfiles(tempDir, ttl)

	// 3. Assert facts
	if _, err := os.Stat(activeProfile); os.IsNotExist(err) {
		t.Errorf("O Sweeper apagou um perfil ativo (recente)!")
	}

	if _, err := os.Stat(unrelatedFolder); os.IsNotExist(err) {
		t.Errorf("O Sweeper apagou uma pasta que não tem o prefixo do argus!")
	}

	if _, err := os.Stat(orphanProfile); !os.IsNotExist(err) {
		t.Errorf("O Sweeper não apagou o perfil órfão antigo!")
	}
}
