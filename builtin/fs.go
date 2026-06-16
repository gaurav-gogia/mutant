package builtin

import (
	"io"
	"os"
	"time"

	"mutant/object"
)

func FsRead(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}
	path, ok := args[0].(*object.String)
	if !ok {
		return newError("argument to `fs_read` must be STRING, got %s", args[0].Type())
	}
	data, err := os.ReadFile(path.Value)
	if err != nil {
		return newError("fs_read: %s", err.Error())
	}
	return stringObj(string(data))
}

func FsWrite(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("wrong number of arguments. got=%d, want=2", len(args))
	}
	path, ok := args[0].(*object.String)
	if !ok {
		return newError("argument 1 to `fs_write` must be STRING, got %s", args[0].Type())
	}
	content, ok := args[1].(*object.String)
	if !ok {
		return newError("argument 2 to `fs_write` must be STRING, got %s", args[1].Type())
	}
	err := os.WriteFile(path.Value, []byte(content.Value), 0644)
	return fsOkOrError(err)
}

func FsAppend(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("wrong number of arguments. got=%d, want=2", len(args))
	}
	path, ok := args[0].(*object.String)
	if !ok {
		return newError("argument 1 to `fs_append` must be STRING, got %s", args[0].Type())
	}
	content, ok := args[1].(*object.String)
	if !ok {
		return newError("argument 2 to `fs_append` must be STRING, got %s", args[1].Type())
	}
	f, err := os.OpenFile(path.Value, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fsOkOrError(err)
	}
	defer f.Close()
	_, err = f.WriteString(content.Value)
	return fsOkOrError(err)
}

func FsDelete(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}
	path, ok := args[0].(*object.String)
	if !ok {
		return newError("argument to `fs_delete` must be STRING, got %s", args[0].Type())
	}
	return fsOkOrError(os.Remove(path.Value))
}

func FsExists(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}
	path, ok := args[0].(*object.String)
	if !ok {
		return newError("argument to `fs_exists` must be STRING, got %s", args[0].Type())
	}
	_, err := os.Stat(path.Value)
	return boolObj(!os.IsNotExist(err))
}

func FsStat(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}
	path, ok := args[0].(*object.String)
	if !ok {
		return newError("argument to `fs_stat` must be STRING, got %s", args[0].Type())
	}
	info, err := os.Stat(path.Value)
	if err != nil {
		return makeHashObject(map[string]object.Object{
			"name":     stringObj(""),
			"size":     intObj(0),
			"is_dir":   boolObj(false),
			"mod_time": stringObj(""),
			"error":    stringObj(err.Error()),
		})
	}
	return makeHashObject(map[string]object.Object{
		"name":     stringObj(info.Name()),
		"size":     intObj(info.Size()),
		"is_dir":   boolObj(info.IsDir()),
		"mod_time": stringObj(info.ModTime().Format(time.RFC3339)),
		"error":    stringObj(""),
	})
}

func FsList(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}
	path, ok := args[0].(*object.String)
	if !ok {
		return newError("argument to `fs_list` must be STRING, got %s", args[0].Type())
	}
	entries, err := os.ReadDir(path.Value)
	if err != nil {
		return newError("fs_list: %s", err.Error())
	}
	elements := make([]object.Object, 0, len(entries))
	for _, entry := range entries {
		info, infoErr := entry.Info()
		size := int64(0)
		if infoErr == nil {
			size = info.Size()
		}
		elements = append(elements, makeHashObject(map[string]object.Object{
			"name":   stringObj(entry.Name()),
			"size":   intObj(size),
			"is_dir": boolObj(entry.IsDir()),
		}))
	}
	return &object.Array{Elements: elements}
}

func FsMkdir(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}
	path, ok := args[0].(*object.String)
	if !ok {
		return newError("argument to `fs_mkdir` must be STRING, got %s", args[0].Type())
	}
	return fsOkOrError(os.MkdirAll(path.Value, 0755))
}

func FsCopy(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("wrong number of arguments. got=%d, want=2", len(args))
	}
	src, ok := args[0].(*object.String)
	if !ok {
		return newError("argument 1 to `fs_copy` must be STRING, got %s", args[0].Type())
	}
	dst, ok := args[1].(*object.String)
	if !ok {
		return newError("argument 2 to `fs_copy` must be STRING, got %s", args[1].Type())
	}

	in, err := os.Open(src.Value)
	if err != nil {
		return fsOkOrError(err)
	}
	defer in.Close()

	out, err := os.Create(dst.Value)
	if err != nil {
		return fsOkOrError(err)
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return fsOkOrError(err)
	}
	return fsOkOrError(out.Close())
}

func FsMove(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("wrong number of arguments. got=%d, want=2", len(args))
	}
	src, ok := args[0].(*object.String)
	if !ok {
		return newError("argument 1 to `fs_move` must be STRING, got %s", args[0].Type())
	}
	dst, ok := args[1].(*object.String)
	if !ok {
		return newError("argument 2 to `fs_move` must be STRING, got %s", args[1].Type())
	}
	return fsOkOrError(os.Rename(src.Value, dst.Value))
}

// fsOkOrError returns a {ok, error} Hash. err may be nil.
func fsOkOrError(err error) object.Object {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	return makeHashObject(map[string]object.Object{
		"ok":    boolObj(err == nil),
		"error": stringObj(errMsg),
	})
}
