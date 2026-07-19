package health

type CheckPlugin interface {
	Register(service *Service) error
	Name() string
}
