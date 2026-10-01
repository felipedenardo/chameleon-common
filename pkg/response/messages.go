package response

const (
	MsgSuccess       = "success"
	MsgCreated       = "created successfully"
	MsgUpdated       = "updated successfully"
	MsgDeleted       = "deleted successfully"
	MsgDataFetched   = "data fetched successfully"

	// Mensagens de erro: chegam à tela de quem usa o sistema, então vão em
	// português e sem detalhe técnico. O detalhe vai para o log.
	MsgInvalidJSON   = "Os dados enviados estão em um formato inválido."
	MsgValidationErr = "Confira os dados informados."
	MsgParamErr      = "Confira os dados informados."
	MsgNotFound      = "Não encontrado."
	MsgInternalErr   = "Algo deu errado do nosso lado. Tente de novo em instantes."
	MsgTimeout       = "A operação demorou demais. Tente de novo em instantes."
	MsgCancelled     = "A requisição foi cancelada."
)
