import { HttpErrorResponse } from '@angular/common/http';

import {
  API_ERROR_MESSAGES,
  extractSafeApiError,
  extractSafeHttpApiError,
} from './api-error.model';

describe('API error mapping', () => {
  it('maps a nested known code and preserves its request ID', () => {
    expect(
      extractSafeApiError({
        error: {
          code: 'VALIDATION_ERROR',
          message: 'Raw backend message',
          details: null,
          request_id: 'req-validation',
        },
      }),
    ).toEqual({
      code: 'VALIDATION_ERROR',
      message: API_ERROR_MESSAGES.VALIDATION_ERROR,
      requestId: 'req-validation',
    });
  });

  it('maps a nested unknown code conservatively and preserves its request ID', () => {
    expect(
      extractSafeApiError({
        error: {
          code: 'INTERNAL_ERROR',
          message: 'Database credentials and stack trace',
          details: { internal: true },
          request_id: 'req-internal',
        },
      }),
    ).toEqual({
      code: 'UNKNOWN_ERROR',
      message: API_ERROR_MESSAGES.UNKNOWN_ERROR,
      requestId: 'req-internal',
    });
  });

  it('uses the conservative fallback for a malformed payload', () => {
    expect(
      extractSafeApiError({ code: 'VALIDATION_ERROR', message: 'Raw backend message' }),
    ).toEqual({
      code: 'UNKNOWN_ERROR',
      message: API_ERROR_MESSAGES.UNKNOWN_ERROR,
    });
  });

  it('maps a status-zero transport failure to service unavailable', () => {
    const error = new HttpErrorResponse({
      error: new ProgressEvent('error'),
      status: 0,
      statusText: 'Unknown Error',
    });

    expect(extractSafeHttpApiError(error)).toEqual({
      code: 'STOCK_SERVICE_UNAVAILABLE',
      message: API_ERROR_MESSAGES.STOCK_SERVICE_UNAVAILABLE,
    });
  });

  it('maps a malformed 503 response to service unavailable', () => {
    const error = new HttpErrorResponse({
      error: null,
      status: 503,
      statusText: 'Service Unavailable',
    });

    expect(extractSafeHttpApiError(error)).toEqual({
      code: 'STOCK_SERVICE_UNAVAILABLE',
      message: API_ERROR_MESSAGES.STOCK_SERVICE_UNAVAILABLE,
    });
  });

  it('preserves a nested request ID when a 503 unknown code uses the transport fallback', () => {
    const error = new HttpErrorResponse({
      error: {
        error: {
          code: 'INTERNAL_ERROR',
          message: 'Raw backend message',
          details: null,
          request_id: 'req-503',
        },
      },
      status: 503,
      statusText: 'Service Unavailable',
    });

    expect(extractSafeHttpApiError(error)).toEqual({
      code: 'STOCK_SERVICE_UNAVAILABLE',
      message: API_ERROR_MESSAGES.STOCK_SERVICE_UNAVAILABLE,
      requestId: 'req-503',
    });
  });

  it('keeps a nested known code authoritative over an HTTP transport fallback', () => {
    const error = new HttpErrorResponse({
      error: {
        error: {
          code: 'VALIDATION_ERROR',
          message: 'Raw backend message',
          details: null,
          request_id: 'req-known',
        },
      },
      status: 503,
      statusText: 'Service Unavailable',
    });

    expect(extractSafeHttpApiError(error)).toEqual({
      code: 'VALIDATION_ERROR',
      message: API_ERROR_MESSAGES.VALIDATION_ERROR,
      requestId: 'req-known',
    });
  });
});
