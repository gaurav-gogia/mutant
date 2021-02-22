package global

import "mutant/object"

// True is the object version of golang native true
var True = &object.Boolean{Value: true}

// False is the object version of golang native false
var False = &object.Boolean{Value: false}

// Null is the object version of golang native null
var Null = &object.Null{}
