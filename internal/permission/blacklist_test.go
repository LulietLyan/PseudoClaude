package permission

import "testing"

func TestBlacklist(t *testing.T) {
	dangerous := []string{
		"rm -rf /",
		"rm -fr ~",
		":(){ :|:& };:",
		"dd if=/dev/zero of=/dev/sda",
		"mkfs.ext4 /dev/sda1",
		"echo x > /dev/disk0",
	}
	for _, command := range dangerous {
		if ok, _ := hitsBlacklist(command); !ok {
			t.Fatalf("expected blacklist hit for %q", command)
		}
	}
	safe := []string{"rm -rf ./build", "git status", "dd if=input of=output.img"}
	for _, command := range safe {
		if ok, pattern := hitsBlacklist(command); ok {
			t.Fatalf("unexpected blacklist hit for %q via %q", command, pattern)
		}
	}
}
