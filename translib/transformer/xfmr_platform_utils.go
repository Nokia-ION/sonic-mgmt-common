package transformer

import (
	"strconv"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
)

// Ported (subset) from nokia-umf translib/transformer/xfmr_utils.go.
// Only the helpers actually referenced by the platform transformers are
// brought over, to avoid pulling in unrelated/conflicting symbols.

type fieldU64LeafPair struct {
	field string
	leaf  **uint64
}

func readAndParseCounters(entry *db.Value, fls []fieldU64LeafPair) error {
	for _, fl := range fls {
		if e := readAndParseCounter(entry, fl.field, fl.leaf); e != nil {
			switch e.(type) {
			case tlerr.NotFoundError:
				continue
			}
			return e
		}
	}
	return nil
}

func readAndParseCounter(entry *db.Value, attr string, counter_val **uint64) error {
	val1, ok := entry.Field[attr]
	if !ok {
		return tlerr.NotFound("Attr " + attr + " missing")
	}
	v, err := strconv.ParseUint(val1, 10, 64)
	if err != nil {
		return err
	}
	*counter_val = &v
	return nil
}

// Extracts a float32 string from the DB entry field. Converts this string to a
// 4 byte binary value compatible with oc:ieeefloat32 format.
func extractFloat32Str(fieldName string, dbEntry *db.Value) ocbinds.Binary {
	redisStr, ok := dbEntry.Field[fieldName]
	if !ok {
		return nil
	}
	base64Str, err := float32StrTo4Bytes(redisStr)
	if err != nil {
		log.V(lvl.ERROR).Infof("Unable to convert field %s to float string. Value was %s. Error %w", fieldName, redisStr, err)
		return nil
	}
	return base64Str
}
