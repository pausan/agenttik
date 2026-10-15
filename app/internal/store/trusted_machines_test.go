package store

import "testing"

func TestTrustedMachines(t *testing.T) {
	s := testStore(t)
	if ok, err := s.VisitTrustedMachine(TrustedMachine{Key: "k1"}); ok || err != nil {
		t.Fatalf("unknown key visited: %v %v", ok, err)
	}
	if err := s.TrustMachine(TrustedMachine{Key: "k1", MachineID: "m1", Name: "laptop", Address: "10.0.0.2"}); err != nil {
		t.Fatal(err)
	}
	list, _ := s.TrustedMachines()
	if len(list) != 1 || list[0].Name != "laptop" || list[0].AddedAt == 0 {
		t.Fatalf("%+v", list)
	}
	added := list[0].AddedAt
	if ok, err := s.VisitTrustedMachine(TrustedMachine{Key: "k1", MachineID: "m1", Name: "laptop 2", Address: "203.0.113.5"}); !ok || err != nil {
		t.Fatalf("visit: %v %v", ok, err)
	}
	if err := s.TrustMachine(TrustedMachine{Key: "k1", MachineID: "m1", Name: "laptop 3", Address: "10.0.0.3"}); err != nil {
		t.Fatal(err)
	}
	list, _ = s.TrustedMachines()
	if len(list) != 1 || list[0].Name != "laptop 3" || list[0].Address != "10.0.0.3" || list[0].AddedAt != added {
		t.Fatalf("%+v", list)
	}
	if ok, _ := s.ForgetTrustedMachine("k1"); !ok {
		t.Fatal("not forgotten")
	}
	if ok, _ := s.ForgetTrustedMachine("k1"); ok {
		t.Fatal("forgotten twice")
	}
	if ok, _ := s.VisitTrustedMachine(TrustedMachine{Key: "k1"}); ok {
		t.Fatal("forgotten key still trusted")
	}
}
