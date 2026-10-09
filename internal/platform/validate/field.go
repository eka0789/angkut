package validate

import "reflect"

// reflectField adalah alias kecil supaya import reflect hanya di satu tempat
// dan file utama tetap ringkas.
type reflectField = reflect.StructField
