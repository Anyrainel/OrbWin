package wsltrust

import "testing"

func TestGuestIssuerSealJoinsActualProducerAndNeverForgetsIssuedWork(t *testing.T) {
	var forged SealedGuestIssuer
	if forged.NoGuestIssued() == nil {
		t.Fatal("empty seal granted zero-issued proof")
	}
	unused := &guestAdmission{}
	seal, err := unused.seal()
	if err != nil || seal.NoGuestIssued() != nil {
		t.Fatal("actual unused producer failed sealing")
	}
	if _, err := unused.begin(); err == nil {
		t.Fatal("sealed producer admitted late native dispatch")
	}
	issued := &guestAdmission{}
	done, err := issued.begin()
	if err != nil {
		t.Fatal(err)
	}
	seal, err = issued.seal()
	if err != nil || seal.NoGuestIssued() == nil {
		t.Fatal("active issuer claimed no guest work")
	}
	done()
	done()
	if seal.NoGuestIssued() == nil {
		t.Fatal("returned Windows operation erased possible guest ownership")
	}
}
