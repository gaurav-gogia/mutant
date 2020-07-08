# mutant

# main
define all cli commands

# generator
generate compiled file and write on disk

# runner
will only run mut files
generate all compiled code and run, does not write compiled file on disk

# security
called by generator post
- gets all the bytecode
adds internal security
returns secure data to generator

- gets binary data of third party binary
adds external security
returns secure data to generator