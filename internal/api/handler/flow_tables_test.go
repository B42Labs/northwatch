package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOVNTableName(t *testing.T) {
	tests := []struct {
		name    string
		tableID int
		want    string
	}{
		{"admission control", 0, "Admission Control"},
		{"acl", 8, "ACL"},
		{"l2 lookup", 17, "L2 Lookup"},
		{"l3 lookup", 19, "L3 Lookup"},
		{"unknown table id", 99, ""},
		{"negative table id", -1, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, OVNTableName(tt.tableID))
		})
	}
}
