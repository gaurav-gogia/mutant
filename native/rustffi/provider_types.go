package rustffi

type provider interface {
	Invoke(request string) (string, error)
}
