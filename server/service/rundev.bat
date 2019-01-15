@ECHO OFF
del /Q /F %GOPATH%\bin\service.exe
go install
%GOPATH%\bin\service.exe --dev