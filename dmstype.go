package godms

import (
	"fmt"
	"strings"

	"github.com/gosnmp/gosnmp"
	"github.com/pkg/errors"
)

const (
	INTEGER        = gosnmp.Integer
	OCTET_STRING   = gosnmp.OctetString
	DISPLAY_STRING = gosnmp.BitString
)

type AccessType string

const (
	READ_ONLY      AccessType = "read-only"
	READ_AND_WRITE AccessType = "read-write"
)

type StatusType string

const (
	MANDATORY StatusType = "mandatory"
)

type Reader interface {
	ObjectType() string
	Syntax() gosnmp.Asn1BER
	Access() string
	Status() string
	Identifier(index int) string
}

type Writer interface {
	ObjectType() string
	Syntax() gosnmp.Asn1BER
	Access() string
	Status() string
	Identifier() string
	WriteIdentifier(input interface{}) gosnmp.SnmpPDU
}
type readOnlyObject struct {
	objectType string
	syntax     gosnmp.Asn1BER
	status     StatusType
	identifier string
}

func (object readOnlyObject) ObjectType() string     { return object.objectType }
func (object readOnlyObject) Syntax() gosnmp.Asn1BER { return object.syntax }
func (object readOnlyObject) Access() string         { return string(READ_ONLY) }
func (object readOnlyObject) Status() string         { return string(object.status) }
func (object readOnlyObject) Identifier(index int) string {
	return fmt.Sprintf("%s.%d", object.identifier, +index)
}

type readAndWriteObject struct {
	objectType string
	syntax     gosnmp.Asn1BER
	status     StatusType
	identifier string
}

func (object readAndWriteObject) ObjectType() string     { return object.objectType }
func (object readAndWriteObject) Syntax() gosnmp.Asn1BER { return object.syntax }
func (object readAndWriteObject) Access() string         { return string(READ_AND_WRITE) }
func (object readAndWriteObject) Status() string         { return string(object.status) }
func (object readAndWriteObject) Identifier(index int) string {
	return fmt.Sprintf("%s.%d", object.identifier, +index)
}
func (object readAndWriteObject) WriteIdentifier(input interface{}, optionsName ...int) (pdu gosnmp.SnmpPDU, err error) {
	var name string
	if len(optionsName) > 0 {
		name = fmt.Sprintf(".%d", optionsName[0])
	} else {
		name = fmt.Sprintf(".0")
	}
	// Todo Check if object syntax matches input type
	pdu = gosnmp.SnmpPDU{
		Value: input,
		Name:  object.identifier + name,
		Type:  object.syntax,
	}
	return
}

func GetSingleOID(dms *gosnmp.GoSNMP, oid string) (result gosnmp.SnmpPDU, err error) {
	packageResult, err := dms.Get([]string{oid})
	if err != nil {
		return result, err
	}
	if packageResult.Variables[0].Value == nil {
		packageResult, err = dms.Get([]string{oid})
		if err != nil {
			return result, err
		}
	}

	if !strings.Contains(packageResult.Variables[0].Name, oid) {
		return result, errors.New(gosnmp.NoSuchName.String())
	}
	return packageResult.Variables[0], nil
}
