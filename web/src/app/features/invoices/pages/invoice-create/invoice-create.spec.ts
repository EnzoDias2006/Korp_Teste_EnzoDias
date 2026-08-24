import { ComponentFixture, TestBed } from '@angular/core/testing';
import { InvoiceCreate } from './invoice-create';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter, Router } from '@angular/router';
import { provideNoopAnimations } from '@angular/platform-browser/animations';
import { environment } from '../../../../../environments/environment';
import { MatSnackBar } from '@angular/material/snack-bar';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { By } from '@angular/platform-browser';

describe('InvoiceCreate', () => {
  let component: InvoiceCreate;
  let fixture: ComponentFixture<InvoiceCreate>;
  let httpTestingController: HttpTestingController;
  let router: Router;
  let snackBar: MatSnackBar;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [InvoiceCreate],
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        provideRouter([]),
        provideNoopAnimations(),
      ],
    }).compileComponents();

    router = TestBed.inject(Router);
    snackBar = TestBed.inject(MatSnackBar);
    httpTestingController = TestBed.inject(HttpTestingController);

    vi.spyOn(router, 'navigate');
    vi.spyOn(snackBar, 'open');

    fixture = TestBed.createComponent(InvoiceCreate);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  afterEach(() => {
    httpTestingController.verify();
  });

  it('should load products on init and initialize one empty item', () => {
    const req = httpTestingController.expectOne(`${environment.stockApiUrl}/products`);
    expect(req.request.method).toBe('GET');
    req.flush([
      { id: 1, code: 'P1', description: 'Product 1', balance: 10, created_at: '', updated_at: '' },
      { id: 2, code: 'P2', description: 'Product 2', balance: 5, created_at: '', updated_at: '' },
    ]);
    fixture.detectChanges();

    expect(component.products().length).toBe(2);
    expect(component.items.length).toBe(1);
    expect(component.isLoadingProducts()).toBe(false);
  });

  it('shows an explicit empty state with a create-product route when no products exist', () => {
    httpTestingController.expectOne(`${environment.stockApiUrl}/products`).flush([]);
    fixture.detectChanges();

    expect(fixture.debugElement.query(By.css('form'))).toBeNull();
    expect(fixture.nativeElement.textContent).toContain(
      'No products are available for this invoice.',
    );

    const createProductLink = fixture.debugElement.query(By.css('a[routerLink="/products/new"]'));
    expect(createProductLink).toBeTruthy();
    expect(createProductLink.nativeElement.textContent).toContain('Create a product');
  });

  it('maps product-load failure, preserves request_id, and clears it before manual retry', () => {
    const firstRequest = httpTestingController.expectOne(`${environment.stockApiUrl}/products`);
    firstRequest.flush(
      {
        error: {
          code: 'STOCK_SERVICE_UNAVAILABLE',
          message: 'Raw backend message',
          request_id: 'req-products-load',
        },
      },
      { status: 503, statusText: 'Service Unavailable' },
    );
    fixture.detectChanges();

    expect(component.productLoadError()).toEqual({
      code: 'STOCK_SERVICE_UNAVAILABLE',
      message: 'O serviço de estoque está indisponível. Tente novamente em instantes.',
      requestId: 'req-products-load',
    });
    expect(fixture.debugElement.query(By.css('form'))).toBeNull();
    expect(fixture.nativeElement.textContent).toContain('req-products-load');

    const retryButton = fixture.debugElement.query(By.css('.error-banner button'));
    retryButton.nativeElement.click();
    fixture.detectChanges();

    expect(component.productLoadError()).toBeNull();
    expect(component.isLoadingProducts()).toBe(true);
    expect(fixture.nativeElement.textContent).not.toContain('req-products-load');
    expect(fixture.debugElement.query(By.css('form'))).toBeNull();

    const retryRequest = httpTestingController.expectOne(`${environment.stockApiUrl}/products`);
    retryRequest.flush([
      { id: 1, code: 'P1', description: 'Product 1', balance: 10, created_at: '', updated_at: '' },
    ]);
    fixture.detectChanges();

    expect(component.isLoadingProducts()).toBe(false);
    expect(fixture.debugElement.query(By.css('form'))).toBeTruthy();
  });

  it('should add and remove items', () => {
    httpTestingController.expectOne(`${environment.stockApiUrl}/products`).flush([]);
    fixture.detectChanges();

    component.addItem();
    expect(component.items.length).toBe(2);

    component.removeItem(0);
    expect(component.items.length).toBe(1);

    // Should not remove the last item
    component.removeItem(0);
    expect(component.items.length).toBe(1);
  });

  it('should validate quantity > 0 and integer', () => {
    httpTestingController.expectOne(`${environment.stockApiUrl}/products`).flush([]);
    fixture.detectChanges();

    const itemForm = component.items.at(0);

    itemForm.patchValue({ productId: '1', quantity: 0 });
    expect(itemForm.invalid).toBe(true);
    expect(itemForm.get('quantity')?.hasError('min')).toBe(true);

    itemForm.patchValue({ quantity: 1.5 });
    expect(itemForm.invalid).toBe(true);
    expect(itemForm.get('quantity')?.hasError('pattern')).toBe(true);

    itemForm.patchValue({ quantity: 1 });
    expect(itemForm.valid).toBe(true);
  });

  it('should validate duplicate products', () => {
    httpTestingController.expectOne(`${environment.stockApiUrl}/products`).flush([]);
    fixture.detectChanges();

    component.addItem();

    component.items.at(0).patchValue({ productId: '1', quantity: 1 });
    component.items.at(1).patchValue({ productId: '1', quantity: 2 });

    expect(component.form.invalid).toBe(true);
    expect(component.items.hasError('duplicateProducts')).toBe(true);

    component.items.at(1).patchValue({ productId: '2', quantity: 2 });
    expect(component.form.valid).toBe(true);
  });

  it('should submit minimal payload with numeric product_id and navigate on success', () => {
    httpTestingController.expectOne(`${environment.stockApiUrl}/products`).flush([]);
    fixture.detectChanges();

    component.items.at(0).patchValue({ productId: '1', quantity: 2 });

    component.onSubmit();
    fixture.detectChanges();

    expect(component.isSaving()).toBe(true);

    const req = httpTestingController.expectOne(`${environment.billingApiUrl}/invoices`);
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({
      items: [{ product_id: 1, quantity: 2 }],
    });

    req.flush({
      id: 100,
      number: 100,
      status: 'OPEN',
      items: [],
      created_at: '',
      closed_at: null,
    });
    fixture.detectChanges();

    expect(component.isSaving()).toBe(false);
    expect(snackBar.open).toHaveBeenCalledWith('Invoice #100 created successfully', 'Close', {
      duration: 3000,
    });
    expect(router.navigate).toHaveBeenCalledWith(['/invoices', '100']);
  });

  it('submits two valid items in one minimal payload', () => {
    httpTestingController.expectOne(`${environment.stockApiUrl}/products`).flush([
      { id: 1, code: 'P1', description: 'Product 1', balance: 10, created_at: '', updated_at: '' },
      { id: 2, code: 'P2', description: 'Product 2', balance: 5, created_at: '', updated_at: '' },
    ]);
    fixture.detectChanges();

    component.addItem();
    component.items.at(0).setValue({ productId: '1', quantity: 2 });
    component.items.at(1).setValue({ productId: '2', quantity: 3 });

    component.onSubmit();

    const request = httpTestingController.expectOne(`${environment.billingApiUrl}/invoices`);
    expect(request.request.body).toEqual({
      items: [
        { product_id: 1, quantity: 2 },
        { product_id: 2, quantity: 3 },
      ],
    });
    request.flush({
      id: 101,
      number: 101,
      status: 'OPEN',
      items: [
        { product_id: 1, product_code: 'P1', product_description: 'Product 1', quantity: 2 },
        { product_id: 2, product_code: 'P2', product_description: 'Product 2', quantity: 3 },
      ],
      created_at: '',
      closed_at: null,
    });

    expect(router.navigate).toHaveBeenCalledWith(['/invoices', '101']);
  });

  it('should handle STOCK_SERVICE_UNAVAILABLE error and preserve form for retry', () => {
    httpTestingController.expectOne(`${environment.stockApiUrl}/products`).flush([]);
    fixture.detectChanges();

    component.items.at(0).patchValue({ productId: '1', quantity: 2 });

    component.onSubmit();
    fixture.detectChanges();

    const req = httpTestingController.expectOne(`${environment.billingApiUrl}/invoices`);
    req.flush(
      {
        error: { code: 'STOCK_SERVICE_UNAVAILABLE', message: 'Service down' },
      },
      { status: 503, statusText: 'Service Unavailable' },
    );

    fixture.detectChanges();

    expect(component.isSaving()).toBe(false);
    expect(component.errorMessage()?.message).toBe(
      'O serviço de estoque está indisponível. Tente novamente em instantes.',
    );
    expect(component.form.getRawValue().items[0].quantity).toBe(2);
  });

  it('should prevent duplicate submission while first request is pending', () => {
    httpTestingController.expectOne(`${environment.stockApiUrl}/products`).flush([]);
    fixture.detectChanges();

    component.items.at(0).patchValue({ productId: '1', quantity: 2 });

    component.onSubmit();
    fixture.detectChanges();

    expect(component.isSaving()).toBe(true);

    component.onSubmit();
    fixture.detectChanges();

    expect(
      httpTestingController.match(
        (req) => req.url === `${environment.billingApiUrl}/invoices` && req.method === 'POST',
      ),
    ).toHaveLength(1);
  });

  it('disables form controls while saving and restores them after failure', () => {
    httpTestingController.expectOne(`${environment.stockApiUrl}/products`).flush([
      {
        id: 1,
        code: 'P1',
        description: 'Product 1',
        balance: 10,
        created_at: '',
        updated_at: '',
      },
    ]);
    component.items.at(0).setValue({ productId: '1', quantity: 2 });

    component.onSubmit();
    fixture.detectChanges();

    expect(component.isSaving()).toBe(true);
    expect(component.form.disabled).toBe(true);
    expect(component.items.at(0).controls.productId.disabled).toBe(true);
    expect(component.items.at(0).controls.quantity.disabled).toBe(true);
    expect(fixture.debugElement.query(By.css('button[type="submit"]')).nativeElement.disabled).toBe(
      true,
    );

    component.onSubmit();
    const requests = httpTestingController.match(
      (request) =>
        request.url === `${environment.billingApiUrl}/invoices` && request.method === 'POST',
    );
    expect(requests).toHaveLength(1);

    requests[0].flush(
      { error: { code: 'VALIDATION_ERROR', message: 'Invalid invoice' } },
      { status: 422, statusText: 'Unprocessable Entity' },
    );

    expect(component.isSaving()).toBe(false);
    expect(component.form.enabled).toBe(true);
    expect(component.items.at(0).controls.productId.enabled).toBe(true);
    expect(component.items.at(0).controls.quantity.enabled).toBe(true);
  });

  it('preserves entered values and allows a successful manual retry after create failure', () => {
    httpTestingController.expectOne(`${environment.stockApiUrl}/products`).flush([
      {
        id: 1,
        code: 'P1',
        description: 'Product 1',
        balance: 10,
        created_at: '',
        updated_at: '',
      },
    ]);
    component.items.at(0).setValue({ productId: '1', quantity: 2 });

    component.onSubmit();
    const firstRequest = httpTestingController.expectOne(`${environment.billingApiUrl}/invoices`);
    firstRequest.flush(
      {
        error: {
          code: 'STOCK_SERVICE_UNAVAILABLE',
          message: 'Stock unavailable',
          request_id: 'req-create-retry',
        },
      },
      { status: 503, statusText: 'Service Unavailable' },
    );

    expect(component.form.getRawValue()).toEqual({
      items: [{ productId: '1', quantity: 2 }],
    });
    expect(component.errorMessage()?.requestId).toBe('req-create-retry');

    component.onSubmit();
    const retryRequest = httpTestingController.expectOne(`${environment.billingApiUrl}/invoices`);
    expect(retryRequest.request.body).toEqual({
      items: [{ product_id: 1, quantity: 2 }],
    });
    retryRequest.flush({
      id: 102,
      number: 102,
      status: 'OPEN',
      items: [{ product_id: 1, product_code: 'P1', product_description: 'Product 1', quantity: 2 }],
      created_at: '',
      closed_at: null,
    });

    expect(component.errorMessage()).toBeNull();
    expect(router.navigate).toHaveBeenCalledWith(['/invoices', '102']);
  });

  it('maps all required creation error codes to distinct messages and preserves request_id', () => {
    httpTestingController.expectOne(`${environment.stockApiUrl}/products`).flush([]);
    fixture.detectChanges();

    component.items.at(0).patchValue({ productId: '1', quantity: 2 });

    const cases = [
      { code: 'VALIDATION_ERROR', fragment: 'Dados inválidos', requestId: 'req-validation' },
      {
        code: 'PRODUCT_CODE_CONFLICT',
        fragment: 'Código de produto',
        requestId: 'req-product-conflict',
      },
      {
        code: 'PRODUCT_NOT_FOUND',
        fragment: 'Produto não encontrado',
        requestId: 'req-product-missing',
      },
      {
        code: 'INVOICE_NOT_FOUND',
        fragment: 'Fatura não encontrada',
        requestId: 'req-invoice-missing',
      },
      {
        code: 'INSUFFICIENT_STOCK',
        fragment: 'Estoque insuficiente',
        requestId: 'req-stock-insufficient',
      },
      {
        code: 'INVOICE_NOT_OPEN',
        fragment: 'não está mais aberta',
        requestId: 'req-invoice-status',
      },
      { code: 'STOCK_SERVICE_UNAVAILABLE', fragment: 'indisponível', requestId: 'req-stock-down' },
      { code: 'IDEMPOTENCY_CONFLICT', fragment: 'idempotência', requestId: 'req-idempotency' },
    ] as const;

    for (const testCase of cases) {
      component.onSubmit();
      const request = httpTestingController.expectOne(`${environment.billingApiUrl}/invoices`);
      request.flush(
        { error: { code: testCase.code, message: testCase.code, request_id: testCase.requestId } },
        { status: testCase.code === 'IDEMPOTENCY_CONFLICT' ? 409 : 400, statusText: 'Error' },
      );

      expect(component.isSaving()).toBe(false);
      expect(component.form.getRawValue().items[0].quantity).toBe(2);
      expect(component.errorMessage()?.code).toBe(testCase.code);
      expect(component.errorMessage()?.message).toContain(testCase.fragment);
      expect(component.errorMessage()?.requestId).toBe(testCase.requestId);
    }
  });
});
