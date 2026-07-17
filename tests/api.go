package tests

import "github.com/theapemachine/symm/tests/mockapi"

/*
MockConn is a test websocket.Conn emulator extracted into a leaf package so
broker tests can import it without cycling through the full tests harness.
*/
type MockConn = mockapi.MockConn

/*
MockPostCall records one REST post issued through a mock transport.
*/
type MockPostCall = mockapi.MockPostCall

/*
MockAPI is a controllable websocket.API harness extracted into a leaf package.
*/
type MockAPI = mockapi.MockAPI

/*
NewMockAPI constructs controllable public and private Conn emulators.
*/
func NewMockAPI() *MockAPI {
	return mockapi.NewMockAPI()
}
