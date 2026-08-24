import { HttpErrorResponse } from '@angular/common/http';

export interface ApiErrorEnvelope {
  error: {
    code: string;
    message: string;
    details?: unknown;
    request_id?: string;
  };
}

export type KnownErrorCode =
  | 'VALIDATION_ERROR'
  | 'PRODUCT_CODE_CONFLICT'
  | 'PRODUCT_NOT_FOUND'
  | 'INVOICE_NOT_FOUND'
  | 'INSUFFICIENT_STOCK'
  | 'INVOICE_NOT_OPEN'
  | 'STOCK_SERVICE_UNAVAILABLE'
  | 'IDEMPOTENCY_CONFLICT';

export interface SafeApiError {
  code: KnownErrorCode | 'UNKNOWN_ERROR';
  message: string;
  requestId?: string;
}

export const API_ERROR_MESSAGES: Readonly<Record<KnownErrorCode | 'UNKNOWN_ERROR', string>> = {
  VALIDATION_ERROR: 'Dados inválidos. Verifique as informações e tente novamente.',
  PRODUCT_CODE_CONFLICT: 'Código de produto já existe. Utilize um código diferente.',
  PRODUCT_NOT_FOUND: 'Produto não encontrado. Atualize a lista de produtos e tente novamente.',
  INVOICE_NOT_FOUND: 'Fatura não encontrada. Atualize os dados para confirmar o status atual.',
  INSUFFICIENT_STOCK: 'Estoque insuficiente para finalizar esta fatura.',
  INVOICE_NOT_OPEN: 'Esta fatura não está mais aberta. Atualize os dados para ver o status atual.',
  STOCK_SERVICE_UNAVAILABLE:
    'O serviço de estoque está indisponível. Tente novamente em instantes.',
  IDEMPOTENCY_CONFLICT:
    'Não foi possível concluir porque esta operação conflita com um registro de idempotência. Atualize os dados antes de tentar novamente; não repitiremos automaticamente.',
  UNKNOWN_ERROR: 'Ocorreu um erro inesperado. Tente novamente mais tarde.',
};

export function extractSafeApiError(error: unknown): SafeApiError {
  const envelope = error as ApiErrorEnvelope;
  const backendError = envelope?.error;
  if (!backendError || typeof backendError !== 'object') {
    return { code: 'UNKNOWN_ERROR', message: API_ERROR_MESSAGES.UNKNOWN_ERROR };
  }

  const requestId =
    typeof backendError.request_id === 'string' && backendError.request_id.trim()
      ? backendError.request_id
      : undefined;
  const knownMessage = API_ERROR_MESSAGES[backendError.code as KnownErrorCode];
  if (!knownMessage) {
    return { code: 'UNKNOWN_ERROR', message: API_ERROR_MESSAGES.UNKNOWN_ERROR, requestId };
  }

  return {
    code: backendError.code as KnownErrorCode,
    message: knownMessage,
    requestId,
  };
}

export function extractSafeHttpApiError(error: unknown): SafeApiError {
  if (!(error instanceof HttpErrorResponse)) {
    return extractSafeApiError(error);
  }

  const backendError = extractSafeApiError(error.error);
  if (backendError.code !== 'UNKNOWN_ERROR') {
    return backendError;
  }

  if (error.status === 0 || error.status === 503) {
    return {
      ...backendError,
      code: 'STOCK_SERVICE_UNAVAILABLE',
      message: API_ERROR_MESSAGES.STOCK_SERVICE_UNAVAILABLE,
    };
  }

  return backendError;
}
