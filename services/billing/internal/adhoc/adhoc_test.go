package adhoc

import (
	"errors"
	"testing"
)

func TestAdjust(t *testing.T) {
	cases := []struct {
		name      string
		bill      Bill
		paid, net int64
		want      Adjustment
		wantErr   error
	}{
		{"prorata setelah lunas -> deposit", Bill{NetIDR: 300000, Status: "paid"}, 300000, 200000,
			Adjustment{NetIDR: 200000, DeltaIDR: -100000, DepositIDR: 100000, PaidIDR: 200000, Status: "paid"}, nil},
		{"naik melebihi dibayar -> partial", Bill{NetIDR: 100000, Status: "paid"}, 100000, 150000,
			Adjustment{NetIDR: 150000, DeltaIDR: 50000, PaidIDR: 100000, Status: "partial"}, nil},
		{"belum dibayar", Bill{NetIDR: 100000, Status: "unpaid"}, 0, 80000,
			Adjustment{NetIDR: 80000, DeltaIDR: -20000, Status: "unpaid"}, nil},
		{"ke nol tanpa bayar = selesai", Bill{NetIDR: 100000, Status: "unpaid"}, 0, 0,
			Adjustment{NetIDR: 0, DeltaIDR: -100000, Status: "paid"}, nil},
		{"negatif", Bill{NetIDR: 100000}, 0, -1, Adjustment{}, ErrNegativeNet},
		{"void", Bill{NetIDR: 100000, Status: "void"}, 0, 50000, Adjustment{}, ErrVoid},
		{"tanpa perubahan", Bill{NetIDR: 100000}, 0, 100000, Adjustment{}, ErrNoChange},
	}
	for _, tc := range cases {
		got, err := Adjust(tc.bill, tc.paid, tc.net)
		if !errors.Is(err, tc.wantErr) || got != tc.want {
			t.Errorf("%s: got %+v %v, want %+v %v", tc.name, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestValidateCreate(t *testing.T) {
	ok := CreateRequest{SchoolID: "s", StudentUserID: "u", SourceModule: "library", SourceRef: "fine-1",
		Items: []Item{{Name: "Denda", AmountIDR: 5000}, {Name: "Ganti buku", AmountIDR: 45000}}, DueDate: "2026-10-31"}
	total, due, err := ValidateCreate(ok)
	if err != nil || total != 50000 || due.Day() != 31 {
		t.Fatalf("%d %v %v", total, due, err)
	}
	bad := []func(r *CreateRequest){
		func(r *CreateRequest) { r.Items = nil },
		func(r *CreateRequest) { r.Items = []Item{{Name: "x", AmountIDR: 0}} },
		func(r *CreateRequest) { r.SourceModule = "canteen" },
		func(r *CreateRequest) { r.SourceRef = "" },
		func(r *CreateRequest) { r.DueDate = "31-10-2026" },
		func(r *CreateRequest) { r.StudentUserID = "" },
	}
	for i, mutate := range bad {
		r := ok
		r.Items = append([]Item(nil), ok.Items...)
		mutate(&r)
		var ve *ValidationError
		if _, _, err := ValidateCreate(r); !errors.As(err, &ve) {
			t.Errorf("case %d: want ValidationError, got %v", i, err)
		}
	}
	if !SameItems(ok.Items, []Item{{Name: "Denda", AmountIDR: 5000}, {Name: "Ganti buku", AmountIDR: 45000}}) || SameItems(ok.Items, ok.Items[:1]) {
		t.Fatal("SameItems")
	}
}
