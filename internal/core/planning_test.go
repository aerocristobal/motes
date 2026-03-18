package core

import (
	"testing"
)

func TestCreateWithParent(t *testing.T) {
	_, mm := setupTestMemory(t)

	parent, err := mm.Create("task", "Epic", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}

	child, err := mm.Create("task", "Story", CreateOpts{Parent: parent.ID})
	if err != nil {
		t.Fatal(err)
	}

	read, err := mm.Read(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if read.Parent != parent.ID {
		t.Errorf("Parent: got %q, want %q", read.Parent, parent.ID)
	}
}

func TestChildren(t *testing.T) {
	_, mm := setupTestMemory(t)

	parent, _ := mm.Create("task", "Epic", CreateOpts{})
	mm.Create("task", "Child 1", CreateOpts{Parent: parent.ID})
	mm.Create("task", "Child 2", CreateOpts{Parent: parent.ID})
	mm.Create("task", "Unrelated", CreateOpts{})

	children, err := mm.Children(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 2 {
		t.Errorf("expected 2 children, got %d", len(children))
	}
}

func TestListFilterByParent(t *testing.T) {
	_, mm := setupTestMemory(t)

	parent, _ := mm.Create("task", "Epic", CreateOpts{})
	mm.Create("task", "Child 1", CreateOpts{Parent: parent.ID})
	mm.Create("task", "Child 2", CreateOpts{Parent: parent.ID})
	mm.Create("task", "Other", CreateOpts{})

	motes, err := mm.List(ListFilters{Parent: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(motes) != 2 {
		t.Errorf("expected 2 filtered by parent, got %d", len(motes))
	}
}

func TestCreateWithAcceptance(t *testing.T) {
	_, mm := setupTestMemory(t)

	m, err := mm.Create("task", "With criteria", CreateOpts{
		Acceptance: []string{"criterion A", "criterion B"},
	})
	if err != nil {
		t.Fatal(err)
	}

	read, _ := mm.Read(m.ID)
	if len(read.Acceptance) != 2 {
		t.Errorf("Acceptance: got %v", read.Acceptance)
	}
	if read.Acceptance[0] != "criterion A" {
		t.Errorf("Acceptance[0]: got %q", read.Acceptance[0])
	}
}

func TestCheckAcceptance(t *testing.T) {
	_, mm := setupTestMemory(t)

	m, _ := mm.Create("task", "Check test", CreateOpts{
		Acceptance: []string{"A", "B", "C"},
	})

	// Toggle criterion 2
	met := []bool{false, true, false}
	err := mm.Update(m.ID, UpdateOpts{
		AcceptanceMet: met,
	})
	if err != nil {
		t.Fatal(err)
	}

	read, _ := mm.Read(m.ID)
	if len(read.AcceptanceMet) != 3 {
		t.Fatalf("AcceptanceMet length: got %d", len(read.AcceptanceMet))
	}
	if !read.AcceptanceMet[1] {
		t.Error("AcceptanceMet[1] should be true")
	}
	if read.AcceptanceMet[0] || read.AcceptanceMet[2] {
		t.Error("AcceptanceMet[0] and [2] should be false")
	}
}

func TestCreateWithSize(t *testing.T) {
	_, mm := setupTestMemory(t)

	m, err := mm.Create("task", "Sized task", CreateOpts{Size: "l"})
	if err != nil {
		t.Fatal(err)
	}

	read, _ := mm.Read(m.ID)
	if read.Size != "l" {
		t.Errorf("Size: got %q, want %q", read.Size, "l")
	}
}

func TestUpdateParent(t *testing.T) {
	_, mm := setupTestMemory(t)

	parent, _ := mm.Create("task", "Parent", CreateOpts{})
	child, _ := mm.Create("task", "Child", CreateOpts{})

	err := mm.Update(child.ID, UpdateOpts{
		Parent: StringPtr(parent.ID),
	})
	if err != nil {
		t.Fatal(err)
	}

	read, _ := mm.Read(child.ID)
	if read.Parent != parent.ID {
		t.Errorf("Parent after update: got %q", read.Parent)
	}
}

func TestIndexRebuildWithParent(t *testing.T) {
	root, mm := setupTestMemory(t)
	im := NewIndexManager(root)

	parent, _ := mm.Create("task", "Parent", CreateOpts{})
	mm.Create("task", "Child", CreateOpts{Parent: parent.ID})

	motes, _ := mm.ReadAllParallel()
	if err := im.Rebuild(motes); err != nil {
		t.Fatal(err)
	}

	idx, _ := im.Load()

	// Check child_of and parent_of edges exist
	hasChildOf := false
	hasParentOf := false
	for _, e := range idx.Edges {
		if e.EdgeType == "child_of" && e.Target == parent.ID {
			hasChildOf = true
		}
		if e.EdgeType == "parent_of" && e.Source == parent.ID {
			hasParentOf = true
		}
	}
	if !hasChildOf {
		t.Error("expected child_of edge in index")
	}
	if !hasParentOf {
		t.Error("expected parent_of edge in index")
	}
}
