package ltv

import "errors"

var (
	ErrInvalidPacket         = errors.New("invalid net packet")
	ErrRepeatRegisterHandler = errors.New("repeat register handler")
	ErrInvalidCommandID      = errors.New("invalid command id")
)
