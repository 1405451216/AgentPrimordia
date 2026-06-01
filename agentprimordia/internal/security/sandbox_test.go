package security

import (
	"testing"
)

func TestACL_AllowAndCheck(t *testing.T) {
	acl := NewACL()
	acl.Allow("agent-1", "/src/", AccessRead|AccessWrite)

	if !acl.Check("agent-1", "/src/main.go", AccessRead) {
		t.Error("agent-1 should have read access to /src/main.go")
	}
	if !acl.Check("agent-1", "/src/sub/file.go", AccessWrite) {
		t.Error("agent-1 should have write access to /src/sub/file.go")
	}
}

func TestACL_Deny(t *testing.T) {
	acl := NewACL()
	acl.Allow("agent-1", "/src/", AccessAll)
	acl.Deny("agent-1", "/src/secret.key")

	if acl.Check("agent-1", "/src/secret.key", AccessRead) {
		t.Error("agent-1 should be denied access to /src/secret.key")
	}
	if !acl.Check("agent-1", "/src/main.go", AccessRead) {
		t.Error("agent-1 should still have access to /src/main.go")
	}
}

func TestACL_WildcardAgent(t *testing.T) {
	acl := NewACL()
	acl.Allow("*", "/public/", AccessRead)

	if !acl.Check("any-agent", "/public/file.txt", AccessRead) {
		t.Error("any agent should have read access to /public/")
	}
	if acl.Check("any-agent", "/public/file.txt", AccessWrite) {
		t.Error("any agent should not have write access to /public/")
	}
}

func TestACL_NoMatch(t *testing.T) {
	acl := NewACL()
	acl.Allow("agent-1", "/src/", AccessRead)

	if acl.Check("agent-2", "/src/main.go", AccessRead) {
		t.Error("agent-2 should not have access")
	}
	if acl.Check("agent-1", "/docs/file.md", AccessRead) {
		t.Error("agent-1 should not have access outside /src/")
	}
}

func TestACL_InsufficientLevel(t *testing.T) {
	acl := NewACL()
	acl.Allow("agent-1", "/src/", AccessRead)

	if acl.Check("agent-1", "/src/main.go", AccessWrite) {
		t.Error("agent-1 should not have write access with only read permission")
	}
}

func TestACL_Reset(t *testing.T) {
	acl := NewACL()
	acl.Allow("agent-1", "/src/", AccessRead)
	acl.Deny("agent-1", "/src/secret")

	acl.Reset()

	if acl.Check("agent-1", "/src/main.go", AccessRead) {
		t.Error("rules should be cleared after reset")
	}
}

func TestSandbox_CanExecute(t *testing.T) {
	sb := NewSandbox(nil)
	sb.AllowCommand("ls")
	sb.AllowCommand("cat")

	if err := sb.CanExecute("agent-1", "ls"); err != nil {
		t.Errorf("ls should be allowed: %v", err)
	}
	if err := sb.CanExecute("agent-1", "rm"); err == nil {
		t.Error("rm should not be allowed")
	}
}

func TestSandbox_BlockCommand(t *testing.T) {
	sb := NewSandbox(nil)
	sb.BlockCommand("rm")
	sb.BlockCommand("sudo")

	if err := sb.CanExecute("agent-1", "rm"); err == nil {
		t.Error("rm should be blocked")
	}
	if err := sb.CanExecute("agent-1", "ls"); err != nil {
		t.Errorf("ls should be allowed: %v", err)
	}
}

func TestSandbox_CanAccess(t *testing.T) {
	acl := NewACL()
	acl.Allow("agent-1", "/src/", AccessRead|AccessWrite)

	sb := NewSandbox(acl)

	if err := sb.CanAccess("agent-1", "/src/main.go", AccessRead); err != nil {
		t.Errorf("should have access: %v", err)
	}
	if err := sb.CanAccess("agent-1", "/etc/passwd", AccessRead); err == nil {
		t.Error("should not have access to /etc/passwd")
	}
}

func TestSandbox_CanAccess_NilACL(t *testing.T) {
	sb := NewSandbox(nil)

	if err := sb.CanAccess("agent-1", "/any/path", AccessAll); err != nil {
		t.Errorf("nil ACL should allow all access: %v", err)
	}
}

func TestSandbox_ValidatePath(t *testing.T) {
	acl := NewACL()
	acl.Allow("agent-1", "/workspace/", AccessAll)
	sb := NewSandbox(acl)

	if err := sb.ValidatePath("agent-1", "/workspace/file.go", AccessRead); err != nil {
		t.Errorf("valid path should pass: %v", err)
	}

	if err := sb.ValidatePath("agent-1", "/workspace/../../../etc/passwd", AccessRead); err == nil {
		t.Error("path traversal should be detected")
	}
}

func TestSandbox_ValidatePath_CleanPath(t *testing.T) {
	sb := NewSandbox(nil)

	if err := sb.ValidatePath("agent-1", "/workspace/./file.go", AccessRead); err != nil {
		t.Errorf("path with ./ should be cleaned and pass: %v", err)
	}
}
