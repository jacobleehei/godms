package dialogs

import (
	"fmt"
	"time"

	"github.com/pkg/errors"

	"github.com/gosnmp/gosnmp"
	ntcip "github.com/jacobleehei/gontcip"
)

/**********************************************************************************************
Controlling the DMS
Standardized dialogs for controlling the DMS that are more complex than simple GETs or SETs are
defined in the following subsections.
**********************************************************************************************/

func ActivatingMessage(
	dms *gosnmp.GoSNMP,
	// 	dmsActivateMessage.0 is a
	// 	structure containing the
	// 	following data:
	//    - duration,
	//    - priority,
	//    - message memory type,
	//    - message number,
	//    - message CRC,
	//    - message source address
	// 	also feel free to See Clause 4.4.6.4 from https://www.ntcip.org/file/2018/11/NTCIP1203v03f.pdf
	duration, priority, messageMemoryType, messageNumber int,
) error {
	if err := dms.Connect(); err != nil {
		return err
	}

	// The management station shall SET dmsActivateMessage.0 to the desired value. This will cause the
	// controller to perform a consistency check on the message. (See Section 4.3.5 for a description of this
	// consistency check.)
	// Note: dmsActivateMessage.0 is a structure that contains the following information: message type
	// (permanent, changeable, blank, etc.), message number, duration, activation priority, a CRC of the
	// message contents, and a network address of the requester.
	var multiStringOnTargetMessageNumber string
	var beaconOnTargetMessageNumber int
	var pixelserviceOnTargetMessageNumber int

	getResults, err := dms.Get([]string{
		ntcip.DmsMessageMultiString.Identifier(messageMemoryType, messageNumber),
		ntcip.DmsMessageBeacon.Identifier(messageMemoryType, messageNumber),
		ntcip.DmsMessagePixelService.Identifier(messageMemoryType, messageNumber),
	})
	if err != nil {
		return errors.Wrap(err, "get dms failed")
	}
	for _, variable := range getResults.Variables {
		switch variable.Name {
		case ntcip.DmsMessageMultiString.Identifier(messageMemoryType, messageNumber):
			multiStringOnTargetMessageNumber = string(variable.Value.([]uint8))
		case ntcip.DmsMessageBeacon.Identifier(messageMemoryType, messageNumber):
			beaconOnTargetMessageNumber = variable.Value.(int)
		case ntcip.DmsMessagePixelService.Identifier(messageMemoryType, messageNumber):
			pixelserviceOnTargetMessageNumber = variable.Value.(int)
		default:
			return errors.New("no avaliable results")
		}
	}

	activeMessageCode, err := EncodeActivateMessageCode(
		multiStringOnTargetMessageNumber, beaconOnTargetMessageNumber, pixelserviceOnTargetMessageNumber,
		messageMemoryType, duration, priority, messageNumber,
		"127.0.0.1",
	)
	if err != nil {
		return errors.Wrap(err, "encode activate message failed")
	}
	activeMessagePDU, err := ntcip.DmsActivateMessage.WriteIdentifier(activeMessageCode)
	if err != nil {
		return errors.Wrap(err, "write activate message object identifier failed")
	}

	setResult, err := dms.Set([]gosnmp.SnmpPDU{activeMessagePDU})
	if err != nil {
		return errors.Wrap(err, "dms set failed")
	}

	if setResult.Error == gosnmp.NoError {
		// If the response indicates 'noError', the message has been activated and the management station
		// shall GET shortErrorStatus.0 to ensure that there are no errors preventing the display of the message
		// (e.g. a 'criticalTemperature' alarm). The management station may then exit the process.
		getResult, err := ntcip.GetSingleOID(dms, ntcip.ShortErrorStatus.Identifier())
		if err != nil {
			return errors.Wrap(err, "dms get next failed")
		}

		if getResult.Value != nil {
			formatResult, err := ntcip.Format(ntcip.ShortErrorStatus, getResult.Value)
			if err != nil {
				return errors.Wrap(err, "format short error startus failed")
			}

			if len(formatResult.([]string)) != 0 {
				return fmt.Errorf("activate message failed: %v", formatResult.([]string))
			}
		}

		return nil
	} else {
		// If the response from Step 2 indicates an error, the message was not activated. The management
		// station shall GET dmsActivateMsgError.0 and dmsActivateErrorMsgCode.0 to determine the type of
		// error.
		// e) If dmsActivateMsgError equals 'syntaxMULTI' then the management station shall GET the following
		// data to determine the error details:
		// 1) dmsMultiSyntaxError.0
		// 2) dmsMultiSyntaxErrorPosition.0
		// f) If dmsActivateMessageError equals “syntaxMULTI(8)” and dmsMultiSyntaxError equals “other(1)”
		// then the management station shall GET dmsMultiOtherErrorDescription.0 to determine the vendor
		// specific error.
		return errors.New("TO-DO") //@todo
	}
}

// Preconditions1:
// The management station shall ensure that the DMS supports the
// desired volatile or changeable message number and the tags
// within the messages.  The management station should not
// attempt this procedures for any other message type.

// Preconditions2:
// The management station shall ensure that there is sufficient
// storage space remaining for the message to be downloaded.
func DefiningMessage(
	dms *gosnmp.GoSNMP,
	messageMemoryType, messageNumber int,
	mutiString, ownerAddress string, priority int,
	beacon, pixelService int,
) error {
	if err := dms.Connect(); err != nil {
		return err
	}

	// The management station shall SET dmsMessageStatus.x.y to 'modifyReq'.
	dmsMessageStatusName := ntcip.DmsMessageStatus.Identifier(messageMemoryType, messageNumber)
	_, err := dms.Set([]gosnmp.SnmpPDU{{
		Value: ntcip.ModifyReq.Int(),
		Name:  dmsMessageStatusName,
		Type:  gosnmp.Integer,
	}})
	if err != nil {
		return errors.Wrap(err, "set message status failed")
	}

	// The management station shall GET dmsMessageStatus.x.y.
	result, err := ntcip.GetSingleOID(dms, dmsMessageStatusName)
	if err != nil {
		return errors.Wrap(err, "get message status failed")
	}

	if result.Value.(int) != ntcip.Modifying.Int() {
		// If the value is not 'modifying', exit the process. In this case, the management station may SET
		// dmsMessageStatus.x.y to 'notUsedReq' and attempt to restart this process from the beginning. (See
		// Section 4.3.4 for a complete description of the Message Table State Machine.)
		return fmt.Errorf("message status parameter returns wrong value: %d. expect: %d", result.Value.(int), ntcip.Modifying.Int())
	}

	// The management station shall SET the following data to the desired values:
	// 1) dmsMessageMultiString.x.y
	// 2) dmsMessageOwner.x.y
	// 3) dmsMessageRunTimePriority.x.y
	_, err = dms.Set(
		[]gosnmp.SnmpPDU{{
			Value: mutiString,
			Name:  ntcip.DmsMessageMultiString.Identifier(messageMemoryType, messageNumber),
			Type:  ntcip.DmsMessageMultiString.Syntax(),
		},
			{
				Value: ownerAddress,
				Name:  ntcip.DmsMessageOwner.Identifier(messageMemoryType, messageNumber),
				Type:  ntcip.DmsMessageOwner.Syntax(),
			},
			{
				Value: priority,
				Name:  ntcip.DmsMessageRunTimePriority.Identifier(messageMemoryType, messageNumber),
				Type:  ntcip.DmsMessageRunTimePriority.Syntax(),
			},
		})
	if err != nil {
		return errors.Wrap(err, "set mutiString failed")
	}

	// (Required step only if Requirement 3.6.6.5 Beacon Activation Flag is selected as Yes in PRL) The
	// management station shall SET dmsMessageBeacon.x.y to the desired value.
	// Note: The response to this request may be a noSuchName error, indicating that the DMS does not
	// support this optional feature. This error will not affect the sequence of this dialog, but the
	// management station should be aware that the CRC will be calculated with this value defaulted to zero
	// (0).
	_, err = dms.Set([]gosnmp.SnmpPDU{{
		Value: beacon,
		Name:  ntcip.DmsMessageBeacon.Identifier(messageMemoryType, messageNumber),
		Type:  ntcip.DmsMessageBeacon.Syntax(),
	}})
	if err != nil {
		return errors.Wrap(err, "set beacon failed")
	}

	// (Required step only if 2.3.2.2.1 Fiber or 2.3.2.2.3 Flip/Shutter is selected as Yes in PRL) The
	// management station shall SET dmsMessagePixelService.x.y to the desired value.
	// Note: The response to this request may be a noSuchName error, indicating that the DMS does not
	// support this optional feature. This error will not affect the sequence of this dialog, but the
	// management station should be aware that the CRC will be calculated with this value defaulted to zero
	// (0).
	_, err = dms.Set([]gosnmp.SnmpPDU{{
		Value: pixelService,
		Name:  ntcip.DmsMessagePixelService.Identifier(messageMemoryType, messageNumber),
		Type:  ntcip.DmsMessagePixelService.Syntax(),
	}})
	if err != nil {
		return errors.Wrap(err, "set pixel service failed")
	}

	// The management station shall SET dmsMessageStatus.x.y to 'validateReq'. This will cause the
	// controller to initiate a consistency check on the message. (See Section 4.3.5 for a description of this
	// consistency check.)
	_, err = dms.Set([]gosnmp.SnmpPDU{{
		Value: ntcip.ValidateReq.Int(),
		Name:  dmsMessageStatusName,
		Type:  gosnmp.Integer,
	}})
	if err != nil {
		return errors.Wrap(err, "set message status failed")
	}

	// The management station shall repeatedly GET dmsMessageStatus.x.y until the value is not
	// 'validating' or a time-out has been reached.
	timeout := 10
	for result.Value.(int) != ntcip.Valid.Int() {
		if timeout == 0 {
			goto GET_VALIDATE_MESSAGE_ERROR
		}
		result, err = ntcip.GetSingleOID(dms, dmsMessageStatusName)
		if err != nil {
			return errors.Wrap(err, "get message status failed")
		}
		time.Sleep(1 * time.Second)
		timeout--
	}
	// If the value is 'valid', exit the process. Otherwise, the management station shall GET
	// dmsValidateMessageError.0 to determine the reason the message was not validated.
	return nil
GET_VALIDATE_MESSAGE_ERROR:

	// If the value is 'syntaxMULTI', the management station shall GET the following data to determine the
	// error details:
	// 1) dmsMultiSyntaxError.0
	// 2) dmsMultiSyntaxErrorPosition.0

	// If the value is 'other', the management station shall GET the following data to determine the error
	// details:
	// 1) dmsMultiOtherErrorDescription.0

	// Where:
	// x = message type
	// y = message number

	// Note: If, at the end of this process, the value of dmsMessageStatus.x.y is 'valid', the message can
	// be activated.
	return errors.New("TO-DO") //@todo
}

type retrievingResult struct {
	DmsMessageMultiString     string
	DmsMessageOwner           string
	DmsMessageRunTimePriority int
	DmsMessageStatus          int // the return shall be 4(Vaild)
	DmsMessageBeacon          int
	DmsMessagePixelService    int
}

// The standardized dialog for a management station to upload a message from the DMS
// (Precondition) The management station shall ensure that the DMS supports the desired message
// type and number.
func RetrievingMessage(
	dms *gosnmp.GoSNMP,
	messageMemoryType, messageNumber int,
) (result retrievingResult, err error) {
	if err = dms.Connect(); err != nil {
		return result, err
	}
	// The management station shall GET the following data:
	// 1) dmsMessageMultiString.x.y
	// 2) dmsMessageOwner.x.y
	// 3) dmsMessageRunTimePriority.x.y
	// 4) dmsMessageStatus.x.y
	var oids = []string{
		ntcip.DmsMessageMultiString.Identifier(messageMemoryType, messageNumber),
		ntcip.DmsMessageOwner.Identifier(messageMemoryType, messageNumber),
		ntcip.DmsMessageRunTimePriority.Identifier(messageMemoryType, messageNumber),
		ntcip.DmsMessageStatus.Identifier(messageMemoryType, messageNumber),
	}

	getResults, err := dms.Get(oids)
	if err != nil {
		return result, errors.Wrapf(err, "get dmsMessageMultiString failed")
	}
	for _, variable := range getResults.Variables {
		switch variable.Name {
		case ntcip.DmsMessageMultiString.Identifier(messageMemoryType, messageNumber):
			result.DmsMessageMultiString = string(variable.Value.([]uint8))
		case ntcip.DmsMessageOwner.Identifier(messageMemoryType, messageNumber):
			result.DmsMessageOwner = string(variable.Value.([]uint8))
		case ntcip.DmsMessageRunTimePriority.Identifier(messageMemoryType, messageNumber):
			result.DmsMessageRunTimePriority = variable.Value.(int)
		case ntcip.DmsMessageStatus.Identifier(messageMemoryType, messageNumber):
			result.DmsMessageStatus = variable.Value.(int)
		}
	}

	// The management station shall GET dmsMessageBeacon.x.y.
	// Note: The response to this request may be a noSuchName error, indicating that the DMS does not
	// support this optional feature. This error will not affect the sequence of this dialog, but the
	// management station should be aware that the CRC will be calculated with this value defaulted to zero
	// (0).
	getResult, _ := ntcip.GetSingleOID(dms, ntcip.DmsMessageBeacon.Identifier(messageMemoryType, messageNumber))
	if err != nil {
		return result, errors.Wrap(err, "get dmsMessageBeacon failed")
	}
	if _, ok := getResult.Value.(int); ok {
		result.DmsMessageBeacon = getResult.Value.(int)
	}
	// The management station shall GET dmsMessagePixelService.x.y.
	// Note: The response to this request may be a noSuchName error, indicating that the DMS does not
	// support this optional feature. This error will not affect the sequence of this dialog, but the
	// management station should be aware that the CRC will be calculated with this value defaulted to zero
	// (0).
	getResult, _ = ntcip.GetSingleOID(dms, ntcip.DmsMessagePixelService.Identifier(messageMemoryType, messageNumber))
	if err != nil {
		return result, errors.Wrap(err, "get dmsMessagePixelService failed")
	}
	if _, ok := getResult.Value.(int); ok {
		result.DmsMessagePixelService = getResult.Value.(int)
	}
	return
}
