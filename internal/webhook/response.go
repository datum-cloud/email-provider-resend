package webhook

import (
	"net/http"
)

func Ok() Response {
	return webhookResponse(http.StatusOK)
}

func BadRequest() Response {
	return webhookResponse(http.StatusBadRequest)
}

func MethodNotAllowed() Response {
	return webhookResponse(http.StatusMethodNotAllowed)
}

func InternalServerError() Response {
	return webhookResponse(http.StatusInternalServerError)
}

func webhookResponse(httpStatus int) Response {
	return Response{
		HttpStatus: httpStatus,
	}
}
