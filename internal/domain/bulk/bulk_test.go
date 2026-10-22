package bulk

import "testing"

func validRow() Row {
	h := 10.0
	return Row{
		RecipientName:  "Budi Santoso",
		RecipientPhone: "081234567890",
		Street:         "Jl. Melati No. 1",
		PostalCode:     "50241",
		WeightKG:       1.5,
		LengthCM:       &h,
	}
}

func TestValidateRowBarisBaikTanpaIssue(t *testing.T) {
	if issues := ValidateRow(validRow(), Options{}); len(issues) != 0 {
		t.Fatalf("issue = %v, ingin kosong", issues)
	}
}

func TestValidateRowMenagolFieldWajib(t *testing.T) {
	r := Row{WeightKG: 0}
	issues := ValidateRow(r, Options{})
	fields := map[string]bool{}
	for _, is := range issues {
		fields[is.Field] = true
	}
	for _, want := range []string{"recipient_name", "recipient_phone", "street", "postal_code", "weight_kg"} {
		if !fields[want] {
			t.Errorf("field %q tidak dilaporkan; issue = %v", want, issues)
		}
	}
}

func TestValidateRowCODButuhLayananNilai(t *testing.T) {
	r := validRow()
	r.CODAmount = 50000
	if issues := ValidateRow(r, Options{}); len(issues) != 0 {
		t.Errorf("COD dengan nilai sah seharusnya valid; issue = %v", issues)
	}
	r.Insurance = true
	r.DeclaredValue = 0
	if issues := ValidateRow(r, Options{}); len(issues) == 0 {
		t.Error("asuransi tanpa nilai barang harus menghasilkan issue")
	}
}

func TestValidateRowBeratDibatasi(t *testing.T) {
	r := validRow()
	r.WeightKG = 150
	issues := ValidateRow(r, Options{MaxWeightKG: 100})
	if len(issues) != 1 || issues[0].Field != "weight_kg" {
		t.Errorf("issue = %v, ingin weight_kg saja", issues)
	}
}

func TestNormalizePhone(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"+62 812-3456-7890", "081234567890", true},
		{"6281234567890", "081234567890", true},
		{"0812 3456 789", "08123456789", true},
		{"021123456", "021123456", true},
		{"12345", "", false},
		{"0812abc7890", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, err := NormalizePhone(c.in)
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("NormalizePhone(%q) = %q, %v; ingin %q", c.in, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Errorf("NormalizePhone(%q) seharusnya gagal", c.in)
		}
	}
}

func TestNormalizePostalCode(t *testing.T) {
	if got, err := NormalizePostalCode(" 50241 "); err != nil || got != "50241" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := NormalizePostalCode("5024"); err == nil {
		t.Error("4 digit harus ditolak")
	}
}

func TestFindDuplicates(t *testing.T) {
	a := validRow()
	b := validRow()               // kembar
	b.RecipientName = "Beda Nama" // nama beda tidak menghapus duplikat
	c := validRow()
	c.Street = "Jl. Melati No. 2"

	dup := FindDuplicates([]Row{a, b, c})
	if len(dup) != 1 {
		t.Fatalf("dup = %v, ingin hanya baris index 1", dup)
	}
	if len(dup[1]) != 1 || dup[1][0].Field != "_row" {
		t.Errorf("issue duplikat salah: %v", dup[1])
	}
}
