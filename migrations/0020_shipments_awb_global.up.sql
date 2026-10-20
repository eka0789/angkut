-- 0020 — Resi unik GLOBAL (TASK-035, pelacakan publik).
--
-- Migrasi 0016 membuat `uq_shipment_awb` = (organization_id, awb): resi unik
-- per organisasi. Itu cukup untuk jalur tenant, tetapi tidak untuk endpoint
-- publik `GET /api/v1/public/track/{awb}` dan halaman berbagi tautan
-- `/track/{awb}` — keduanya hanya menerima resi tanpa konteks organisasi, jadi
-- sebuah resi HARUS menunjuk tepat SATU pengiriman di seluruh sistem.
--
-- Tanpa index ini, dua organisasi yang mengirim lewat kurir yang sama pada hari
-- yang sama bisa menerima resi identik (urutan dihitung per organisasi), dan
-- pelacakan publik menjadi ambigu — bisa menampilkan pengiriman milik pihak
-- yang salah.
--
-- Index 0016 TETAP dipertahankan: jalur tenant tetap diperiksa dua kali, dan
-- dokumentasi "AWB unik per org" tetap benar (lebih ketat lagi sekarang).
--
-- Isi kolom `awb` sudah diperiksa tidak ada duplikat lintas organisasi sebelum
-- index ini dibuat; bila ternyata ada, index gagal dibuat dan duplikat harus
-- diselesaikan lebih dulu (jangan memaksa).

CREATE UNIQUE INDEX uq_shipment_awb_global ON shipments (awb);

COMMENT ON INDEX uq_shipment_awb_global IS
    'Resi unik di seluruh sistem — prasyarat pelacakan publik per-resi (TASK-035).';
