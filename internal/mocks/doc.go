/*
Package mocks will have all the mocks of the application, we'll try to use mocking using blackbox
testing and integration tests whenever is possible.
*/
package mocks // import "github.com/spotahome/gontroller/internal/mocks"

//go:generate mockery -output ./controller -outpkg controller -dir ../../controller -name ListerWatcher
//go:generate mockery -output ./controller -outpkg controller -dir ../../controller -name Handler
//go:generate mockery -output ./controller -outpkg controller -dir ../../controller -name Storage
