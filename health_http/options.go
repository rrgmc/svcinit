package health_http

type HandlerOption interface {
	handlerOption
	serverOption
}

type ServerOption interface {
	serverOption
}

type handlerOption interface {
	applyHandlerOption(*Handler)
}

type serverOption interface {
	applyServerOption(*Server)
}

// internal

type optionImpl struct {
	handlerOpt func(*Handler)
	serverOpt  func(*Server)
}

func (o *optionImpl) applyHandlerOption(handler *Handler) {
	if o.handlerOpt != nil {
		o.handlerOpt(handler)
	}
}

func (o *optionImpl) applyServerOption(server *Server) {
	if o.serverOpt != nil {
		o.serverOpt(server)
	}
}
