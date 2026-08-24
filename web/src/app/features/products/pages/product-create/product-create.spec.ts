import { HttpErrorResponse } from '@angular/common/http';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';
import { Router, provideRouter } from '@angular/router';
import { Observable, Subject, of, throwError } from 'rxjs';
import { vi } from 'vitest';

import { ApiErrorEnvelope } from '../../../../core/models/api-error.model';
import { CreateProductRequest, Product } from '../../../../core/models/product.model';
import { ProductApiService } from '../../services/product-api.service';
import { ProductCreate } from './product-create';

const createdProduct: Product = {
  id: '1',
  code: 'TEST',
  description: 'Test',
  balance: 10,
  createdAt: '2024-01-01',
  updatedAt: '2024-01-01',
};

class MockProductApiService {
  readonly create = vi.fn<(request: CreateProductRequest) => Observable<Product>>(() =>
    of(createdProduct),
  );
}

describe('ProductCreate', () => {
  let component: ProductCreate;
  let fixture: ComponentFixture<ProductCreate>;
  let mockProductApi: MockProductApiService;
  let router: Router;

  beforeEach(async () => {
    mockProductApi = new MockProductApiService();
    await TestBed.configureTestingModule({
      imports: [ProductCreate, NoopAnimationsModule],
      providers: [provideRouter([]), { provide: ProductApiService, useValue: mockProductApi }],
    }).compileComponents();

    router = TestBed.inject(Router);
    vi.spyOn(router, 'navigate').mockResolvedValue(true);
    fixture = TestBed.createComponent(ProductCreate);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  function setValidForm(): void {
    component.form.setValue({
      code: 'PROD001',
      description: 'Product Description',
      balance: 10,
    });
  }

  it('creates the component with the expected initial form values', () => {
    expect(component).toBeTruthy();
    expect(component.form.getRawValue()).toEqual({
      code: '',
      description: '',
      balance: 0,
    });
  });

  it('requires code', () => {
    component.code.setValue('');
    component.code.markAsTouched();

    expect(component.code.invalid).toBe(true);
    expect(component.code.hasError('required')).toBe(true);
  });

  it('requires description', () => {
    component.description.setValue('');
    component.description.markAsTouched();

    expect(component.description.invalid).toBe(true);
    expect(component.description.hasError('required')).toBe(true);
  });

  it('requires balance', () => {
    component.balance.setValue(null);
    component.balance.markAsTouched();

    expect(component.balance.invalid).toBe(true);
    expect(component.balance.hasError('required')).toBe(true);
  });

  it('rejects whitespace-only code', () => {
    component.code.setValue('   ');
    component.code.markAsTouched();

    expect(component.code.hasError('pattern')).toBe(true);
  });

  it('rejects whitespace-only description', () => {
    component.description.setValue('   ');
    component.description.markAsTouched();

    expect(component.description.hasError('pattern')).toBe(true);
  });

  it('rejects negative balance', () => {
    component.balance.setValue(-1);
    component.balance.markAsTouched();

    expect(component.balance.hasError('min')).toBe(true);
  });

  it('rejects fractional balance', () => {
    component.balance.setValue(1.5);
    component.balance.markAsTouched();

    expect(component.balance.hasError('notInteger')).toBe(true);
  });

  it('accepts valid form values', () => {
    setValidForm();

    expect(component.form.valid).toBe(true);
  });

  it('does not invent text length limits absent from the backend contract', () => {
    component.form.setValue({
      code: 'P'.repeat(51),
      description: 'Product description '.repeat(11),
      balance: 0,
    });

    expect(component.form.valid).toBe(true);
  });

  it('disables the submit button when the form is invalid', () => {
    component.code.setValue('');
    fixture.detectChanges();

    const submitButton = fixture.nativeElement.querySelector(
      'button[type="submit"]',
    ) as HTMLButtonElement;
    expect(submitButton.disabled).toBe(true);
  });

  it('disables the submit button while saving', () => {
    setValidForm();
    component.saving.set(true);
    fixture.detectChanges();

    const submitButton = fixture.nativeElement.querySelector(
      'button[type="submit"]',
    ) as HTMLButtonElement;
    expect(submitButton.disabled).toBe(true);
  });

  it('calls create with trimmed values on valid submit', () => {
    component.form.setValue({
      code: '  PROD001  ',
      description: '  Product Description  ',
      balance: 10,
    });

    component.onSubmit();

    expect(mockProductApi.create).toHaveBeenCalledTimes(1);
    expect(mockProductApi.create).toHaveBeenCalledWith({
      code: 'PROD001',
      description: 'Product Description',
      balance: 10,
    });
  });

  it('shows saving state while the request is pending', () => {
    const response = new Subject<Product>();
    mockProductApi.create.mockReturnValue(response);
    setValidForm();

    component.onSubmit();

    expect(component.saving()).toBe(true);
    expect(component.submitInFlight()).toBe(true);

    response.complete();
  });

  it('navigates only after the backend confirms product creation', () => {
    setValidForm();

    component.onSubmit();

    expect(component.errorMessage()).toBeNull();
    expect(router.navigate).toHaveBeenCalledWith(['/products']);
    expect(component.saving()).toBe(false);
    expect(component.submitInFlight()).toBe(false);
  });

  it('prevents a second submit while the first request is pending', () => {
    const response = new Subject<Product>();
    mockProductApi.create.mockReturnValue(response);
    setValidForm();

    component.onSubmit();
    component.onSubmit();

    expect(mockProductApi.create).toHaveBeenCalledTimes(1);

    response.complete();
  });

  it('shows a safe validation message and preserves form values', () => {
    mockProductApi.create.mockReturnValue(
      throwError(() => createHttpError('VALIDATION_ERROR', 'Validation failed', 422)),
    );
    setValidForm();

    component.onSubmit();

    expect(component.errorMessage()?.code).toBe('VALIDATION_ERROR');
    expect(component.errorMessage()?.message).toContain('Dados inválidos');
    expect(component.form.getRawValue()).toEqual({
      code: 'PROD001',
      description: 'Product Description',
      balance: 10,
    });
    expect(router.navigate).not.toHaveBeenCalled();
  });

  it('shows a safe product-code conflict message and preserves form values', () => {
    mockProductApi.create.mockReturnValue(
      throwError(() => createHttpError('PRODUCT_CODE_CONFLICT', 'Code already exists', 409)),
    );
    setValidForm();

    component.onSubmit();

    expect(component.errorMessage()?.code).toBe('PRODUCT_CODE_CONFLICT');
    expect(component.code.value).toBe('PROD001');
    expect(router.navigate).not.toHaveBeenCalled();
  });

  it('shows a safe unavailable-service message for a connectivity error', () => {
    mockProductApi.create.mockReturnValue(
      throwError(
        () =>
          new HttpErrorResponse({
            error: {},
            status: 0,
            statusText: 'Unknown Error',
          }),
      ),
    );
    setValidForm();

    component.onSubmit();

    expect(component.errorMessage()?.code).toBe('STOCK_SERVICE_UNAVAILABLE');
    expect(component.code.value).toBe('PROD001');
    expect(router.navigate).not.toHaveBeenCalled();
  });

  it('preserves the request ID from a 503 unavailable-service response', () => {
    mockProductApi.create.mockReturnValue(
      throwError(() =>
        createHttpError(
          'STOCK_SERVICE_UNAVAILABLE',
          'Could not create product',
          503,
          'req-product-503',
        ),
      ),
    );
    setValidForm();

    component.onSubmit();

    expect(component.errorMessage()).toEqual({
      code: 'STOCK_SERVICE_UNAVAILABLE',
      message: 'O serviço de estoque está indisponível. Tente novamente em instantes.',
      requestId: 'req-product-503',
    });
  });

  it('does not render malformed backend message or request ID values', () => {
    mockProductApi.create.mockReturnValue(
      throwError(
        () =>
          new HttpErrorResponse({
            error: {
              error: {
                code: 'UNKNOWN_ERROR',
                message: 'database password leaked',
                request_id: { internal: 'secret' },
              },
            },
            status: 500,
          }),
      ),
    );
    setValidForm();

    component.onSubmit();

    expect(component.errorMessage()?.code).toBe('UNKNOWN_ERROR');
    expect(component.errorMessage()?.requestId).toBeUndefined();
  });

  it('preserves a valid request ID as secondary diagnostic data', () => {
    mockProductApi.create.mockReturnValue(
      throwError(() => createHttpError('VALIDATION_ERROR', 'Validation failed', 422, 'req-123')),
    );
    setValidForm();

    component.onSubmit();

    expect(component.errorMessage()?.code).toBe('VALIDATION_ERROR');
    expect(component.errorMessage()?.requestId).toBe('req-123');
  });

  it('marks invalid controls as touched without issuing a request', () => {
    component.form.setValue({ code: '', description: '', balance: -1 });

    component.onSubmit();

    expect(component.code.touched).toBe(true);
    expect(component.description.touched).toBe(true);
    expect(component.balance.touched).toBe(true);
    expect(mockProductApi.create).not.toHaveBeenCalled();
  });
});

function createHttpError(
  code: string,
  message: string,
  status: number,
  requestId?: string,
): HttpErrorResponse {
  const error: ApiErrorEnvelope = {
    error: {
      code,
      message,
      details: null,
      ...(requestId ? { request_id: requestId } : {}),
    },
  };

  return new HttpErrorResponse({ error, status });
}
