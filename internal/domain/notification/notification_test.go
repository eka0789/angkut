package notification

import "testing"

func TestRenderMenggantiPlaceholder(t *testing.T) {
	out := Render("Resi {{awb}} untuk {{name}}", map[string]any{
		"awb":  "JNE260101000001",
		"name": "Budi",
	})
	if out != "Resi JNE260101000001 untuk Budi" {
		t.Errorf("out = %q", out)
	}
}

func TestRenderVariabelTidakDikenalDibiarkan(t *testing.T) {
	out := Render("Resi {{awb}} / {{unknown}}", map[string]any{"awb": "X1"})
	if out != "Resi X1 / {{unknown}}" {
		t.Errorf("out = %q, ingin placeholder tak dikenal tetap utuh", out)
	}
}

func TestRenderPlaceholderPanjangTidakDitimpaPendek(t *testing.T) {
	// {{awb_label}} harus terisi penuh; bila {{awb}} lebih dulu, hasilnya
	// akan menjadi "X_suffix" — itu bug rendering yang harus dihindari.
	out := Render("{{awb_label}}", map[string]any{"awb": "X", "awb_label": "LABEL"})
	if out != "LABEL" {
		t.Errorf("out = %q, ingin LABEL", out)
	}
}

func TestTemplateForEventTidakDikenal(t *testing.T) {
	if _, ok := TemplateFor(EventKey("tidak.ada"), ChannelInApp); ok {
		t.Error("event tak dikenal seharusnya tidak punya template")
	}
	if _, ok := TemplateFor(EventShipmentDelivered, ChannelInApp); !ok {
		t.Error("event shipment.delivered harus punya template")
	}
}

func TestPriorityUntukEventKritis(t *testing.T) {
	if PriorityFor(EventShipmentFailed) != PriorityHigh {
		t.Error("shipment.failed harus prioritas tinggi")
	}
	if PriorityFor(EventShipmentCreated) != PriorityLow {
		t.Error("shipment.created harus prioritas rendah")
	}
}

func TestValidEventKeyDanChannel(t *testing.T) {
	for _, k := range EventKeys() {
		if !k.Valid() {
			t.Errorf("%q harus valid", k)
		}
	}
	if EventKey("x.y").Valid() {
		t.Error("event tak dikenal harus tidak valid")
	}
	if !ChannelInApp.Valid() || !ChannelEmail.Valid() {
		t.Error("kanal inapp/email harus valid")
	}
	if Channel("whatsapp").Valid() {
		t.Error("kanal whatsapp belum didukung di TASK-040")
	}
}
