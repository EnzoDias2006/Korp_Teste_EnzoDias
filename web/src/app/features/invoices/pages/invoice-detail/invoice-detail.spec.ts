import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ActivatedRoute } from '@angular/router';
import { By } from '@angular/platform-browser';
import { BehaviorSubject } from 'rxjs';
import { InvoiceDetail } from './invoice-detail';
import { environment } from '../../../../../environments/environment';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';
import { MatProgressSpinner } from '@angular/material/progress-spinner';
import { DatePipe } from '@angular/common';
import { MatSnackBarModule } from '@angular/material/snack-bar';
import { Invoice } from '../../../../core/models/invoice.model';
import { InvoiceApiService } from '../../services/invoice-api.service';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

describe('InvoiceDetail', () => {
  let component: InvoiceDetail;
  let fixture: ComponentFixture<InvoiceDetail>;
  let httpTestingController: HttpTestingController;
  let paramMapSubject: BehaviorSubject<{ get: (name: string) => string | null }>;
  let windowPrintCalls: number[] = [];

  beforeEach(() => {
    windowPrintCalls = [];
    window.print = () => {
      windowPrintCalls.push(1);
    };
  });

  const closedInvoice: Invoice = {
    id: '123',
    number: 1001,
    status: 'CLOSED',
    items: [
      { productId: '10', productCode: 'PROD-A', productDescription: 'Product A', quantity: 2 },
    ],
    createdAt: '2026-08-21T10:00:00Z',
    closedAt: '2026-08-21T10:05:00Z',
  };

  beforeEach(async () => {
    paramMapSubject = new BehaviorSubject<{ get: (name: string) => string | null }>({
      get: () => '123',
    });

    await TestBed.configureTestingModule({
      imports: [InvoiceDetail, NoopAnimationsModule, MatSnackBarModule],
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        {
          provide: ActivatedRoute,
          useValue: {
            paramMap: paramMapSubject.asObservable(),
          },
        },
        InvoiceApiService,
        DatePipe,
      ],
    }).compileComponents();

    httpTestingController = TestBed.inject(HttpTestingController);
    fixture = TestBed.createComponent(InvoiceDetail);
    component = fixture.componentInstance;
  });

  afterEach(() => {
    httpTestingController.verify();
    vi.clearAllMocks();
  });

  it('should show loading spinner initially', () => {
    fixture.detectChanges();
    const spinner = fixture.debugElement.query(By.directive(MatProgressSpinner));
    expect(spinner).toBeTruthy();
    expect(component.loading()).toBe(true);

    // Clear the pending request
    const req = httpTestingController.expectOne(`${environment.billingApiUrl}/invoices/123`);
    req.flush({});
  });

  it('should display invoice details on success', () => {
    fixture.detectChanges();
    const req = httpTestingController.expectOne(`${environment.billingApiUrl}/invoices/123`);
    expect(req.request.method).toBe('GET');

    req.flush({
      id: 123,
      number: 1001,
      status: 'OPEN',
      created_at: '2026-08-21T10:00:00Z',
      closed_at: null,
      items: [
        {
          product_id: 10,
          product_code: 'PROD-A',
          product_description: 'Product A Description',
          quantity: 2,
        },
        {
          product_id: 11,
          product_code: 'PROD-B',
          product_description: 'Product B Description',
          quantity: 5,
        },
      ],
    });

    fixture.detectChanges();

    expect(component.loading()).toBe(false);
    expect(component.invoice()).toBeTruthy();
    expect(component.invoice()?.number).toBe(1001);

    const html = fixture.nativeElement.innerHTML;
    expect(html).toContain('Fatura #1001');
    expect(html).toContain('Aberta');
    expect(html).toContain('PROD-A');
    expect(html).toContain('Product A Description');
    expect(html).toContain('2');
    expect(html).toContain('PROD-B');
    expect(html).toContain('Product B Description');
    expect(html).toContain('5');

    httpTestingController.expectNone((r) => r.url.includes('stock') || r.url.includes('products'));
  });

  it('should show not found message when API returns 404', () => {
    fixture.detectChanges();
    const req = httpTestingController.expectOne(`${environment.billingApiUrl}/invoices/123`);
    req.flush('Not Found', { status: 404, statusText: 'Not Found' });

    fixture.detectChanges();

    expect(component.loading()).toBe(false);
    expect(component.notFound()).toBe(true);
    const html = fixture.nativeElement.innerHTML;
    expect(html).toContain('Fatura não encontrada');
  });

  it('should show error message when API fails and allow retry', () => {
    fixture.detectChanges();
    const req = httpTestingController.expectOne(`${environment.billingApiUrl}/invoices/123`);
    req.flush('Server Error', { status: 500, statusText: 'Server Error' });

    fixture.detectChanges();

    expect(component.loading()).toBe(false);
    expect(component.error()).toBeTruthy();
    const html = fixture.nativeElement.innerHTML;
    expect(html).toContain('Erro de comunicação');

    // Retry
    component.retry();
    fixture.detectChanges();

    expect(component.loading()).toBe(true);
    const reqRetry = httpTestingController.expectOne(`${environment.billingApiUrl}/invoices/123`);
    reqRetry.flush({
      id: 123,
      number: 1001,
      status: 'OPEN',
      created_at: '2026-08-21T10:00:00Z',
      closed_at: null,
      items: [],
    });

    fixture.detectChanges();
    expect(component.loading()).toBe(false);
    expect(component.error()).toBeNull();
    expect(component.invoice()?.number).toBe(1001);
  });

  describe('print functionality', () => {
    beforeEach(() => {
      fixture.detectChanges();
      const req = httpTestingController.expectOne(`${environment.billingApiUrl}/invoices/123`);
      req.flush({
        id: 123,
        number: 1001,
        status: 'OPEN',
        created_at: '2026-08-21T10:00:00Z',
        closed_at: null,
        items: [
          { product_id: 10, product_code: 'PROD-A', product_description: 'Product A', quantity: 2 },
        ],
      });
      fixture.detectChanges();
    });

    it('canPrint is true for OPEN invoice and not printing', () => {
      expect(component.invoice()?.status).toBe('OPEN');
      expect(component.printing()).toBe(false);
      expect(component.canPrint()).toBe(true);
    });

    it('canPrint is false for CLOSED invoice', () => {
      component.invoice.set(closedInvoice);
      fixture.detectChanges();
      expect(component.canPrint()).toBe(false);
    });

    it('canPrint is false while printing is true', () => {
      component.printing.set(true);
      fixture.detectChanges();
      expect(component.canPrint()).toBe(false);
    });

    it('canPrint is false when invoice is null', () => {
      component.invoice.set(null);
      fixture.detectChanges();
      expect(component.canPrint()).toBe(false);
    });

    it('print() does nothing when canPrint is false for CLOSED invoice', () => {
      component.invoice.set(closedInvoice);
      fixture.detectChanges();
      expect(component.canPrint()).toBe(false);
      component.print();
      httpTestingController.expectNone(`${environment.billingApiUrl}/invoices/123/print`);
    });

    it('print() does nothing when canPrint is false for null invoice', () => {
      component.invoice.set(null);
      fixture.detectChanges();
      component.print();
      httpTestingController.expectNone(`${environment.billingApiUrl}/invoices/123/print`);
    });

    it('print() calls POST /invoices/:id/print and sets printing true', () => {
      expect(component.canPrint()).toBe(true);
      component.print();
      expect(component.printing()).toBe(true);
      const req = httpTestingController.expectOne(
        `${environment.billingApiUrl}/invoices/123/print`,
      );
      expect(req.request.method).toBe('POST');
      expect(req.request.body).toBeNull();
    });

    it('on successful print updates invoice to CLOSED with closedAt', async () => {
      component.print();
      const req = httpTestingController.expectOne(
        `${environment.billingApiUrl}/invoices/123/print`,
      );
      req.flush({
        id: 123,
        number: 1001,
        status: 'CLOSED',
        created_at: '2026-08-21T10:00:00Z',
        closed_at: '2026-08-21T10:05:00Z',
        items: [
          { product_id: 10, product_code: 'PROD-A', product_description: 'Product A', quantity: 2 },
        ],
      });
      await fixture.whenStable();

      expect(component.invoice()?.status).toBe('CLOSED');
      expect(component.invoice()?.closedAt).toBe('2026-08-21T10:05:00Z');
      expect(component.printing()).toBe(false);
      expect(windowPrintCalls.length).toBe(1);
    });

    it('printing flag is cleared on error', async () => {
      component.print();
      const req = httpTestingController.expectOne(
        `${environment.billingApiUrl}/invoices/123/print`,
      );
      req.flush('Stock service unavailable', { status: 503, statusText: 'Service Unavailable' });
      await fixture.whenStable();

      expect(component.printing()).toBe(false);
    });

    it('INSUFFICIENT_STOCK error sets actionError and preserves OPEN invoice', async () => {
      component.print();
      const req = httpTestingController.expectOne(
        `${environment.billingApiUrl}/invoices/123/print`,
      );
      req.flush(
        { error: { code: 'INSUFFICIENT_STOCK', message: 'Insufficient stock' } },
        { status: 409, statusText: 'Conflict' },
      );
      await fixture.whenStable();

      expect(component.actionError()?.message).toBe(
        'Estoque insuficiente para finalizar esta fatura.',
      );
      expect(component.invoice()?.status).toBe('OPEN');
      expect(component.printing()).toBe(false);
    });

    it('INVOICE_NOT_OPEN error sets actionError, clears it, and reloads', async () => {
      component.print();
      const printReq = httpTestingController.expectOne(
        `${environment.billingApiUrl}/invoices/123/print`,
      );
      printReq.flush(
        { error: { code: 'INVOICE_NOT_OPEN', message: 'Invoice not open' } },
        { status: 409, statusText: 'Conflict' },
      );

      const reloadReq = httpTestingController.expectOne(
        `${environment.billingApiUrl}/invoices/123`,
      );
      reloadReq.flush({
        id: 123,
        number: 1001,
        status: 'OPEN',
        created_at: '2026-08-21T10:00:00Z',
        closed_at: null,
        items: [],
      });

      await fixture.whenStable();

      expect(component.actionError()?.message).toBe(
        'Esta fatura não está mais aberta. Atualize os dados para ver o status atual.',
      );
      expect(component.invoice()?.status).toBe('OPEN');
    });

    it('malformed error sets conservative fallback message', async () => {
      component.print();
      const req = httpTestingController.expectOne(
        `${environment.billingApiUrl}/invoices/123/print`,
      );
      req.flush(null, { status: 500, statusText: 'Internal Server Error' });
      await fixture.whenStable();

      expect(component.actionError()?.message).toBe(
        'Ocorreu um erro inesperado. Tente novamente mais tarde.',
      );
    });

    it('maps all required print error codes to distinct safe messages and preserves request ids', async () => {
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
        { code: 'IDEMPOTENCY_CONFLICT', fragment: 'idempotência', requestId: 'req-idempotency' },
      ] as const;

      for (const testCase of cases) {
        component.print();
        const request = httpTestingController.expectOne(
          `${environment.billingApiUrl}/invoices/123/print`,
        );
        request.flush(
          {
            error: { code: testCase.code, message: testCase.code, request_id: testCase.requestId },
          },
          { status: 400, statusText: 'Bad Request' },
        );
        await fixture.whenStable();

        expect(component.actionError()?.code).toBe(testCase.code);
        expect(component.actionError()?.message).toContain(testCase.fragment);
        expect(component.actionError()?.requestId).toBe(testCase.requestId);
        expect(component.invoice()?.status).toBe('OPEN');

        component.actionError.set(null);
      }
    });

    it('simultaneous print clicks are prevented by canPrint guard', () => {
      component.print();
      expect(component.printing()).toBe(true);
      component.print();
      component.print();
      expect(component.printing()).toBe(true);
      const req = httpTestingController.expectOne(
        `${environment.billingApiUrl}/invoices/123/print`,
      );
      req.flush({
        id: 123,
        number: 1001,
        status: 'CLOSED',
        created_at: '2026-08-21T10:00:00Z',
        closed_at: '2026-08-21T10:05:00Z',
        items: [],
      });
    });

    it('window.print is called only on success, not on error', async () => {
      component.print();
      const req = httpTestingController.expectOne(
        `${environment.billingApiUrl}/invoices/123/print`,
      );
      req.flush(
        { error: { code: 'INSUFFICIENT_STOCK', message: 'Insufficient stock' } },
        { status: 409, statusText: 'Conflict' },
      );
      await fixture.whenStable();

      expect(windowPrintCalls.length).toBe(0);
    });

    it('does not show CLOSED or call window.print for a malformed success payload', async () => {
      component.print();
      const req = httpTestingController.expectOne(
        `${environment.billingApiUrl}/invoices/123/print`,
      );
      req.flush({
        id: 123,
        number: 1001,
        status: 'CLOSED',
        created_at: '2026-08-21T10:00:00Z',
        closed_at: null,
        items: [],
      });
      await fixture.whenStable();

      expect(component.invoice()?.status).toBe('OPEN');
      expect(component.printing()).toBe(false);
      expect(component.actionError()?.code).toBe('UNKNOWN_ERROR');
      expect(windowPrintCalls.length).toBe(0);
    });

    it('STOCK_SERVICE_UNAVAILABLE preserves OPEN state and a retry sends another POST', async () => {
      component.print();
      const firstRequest = httpTestingController.expectOne(
        `${environment.billingApiUrl}/invoices/123/print`,
      );
      firstRequest.flush(
        {
          error: {
            code: 'STOCK_SERVICE_UNAVAILABLE',
            message: 'Stock unavailable',
            request_id: 'req-stock',
          },
        },
        { status: 503, statusText: 'Service Unavailable' },
      );
      await fixture.whenStable();

      expect(component.invoice()?.status).toBe('OPEN');
      expect(component.printing()).toBe(false);
      expect(component.actionError()?.message).toContain('indisponível');
      expect(component.actionError()?.requestId).toBe('req-stock');
      fixture.detectChanges();
      const visibleRequestId = fixture.debugElement.query(By.css('.action-request-id code'));
      expect(visibleRequestId).toBeTruthy();
      expect(visibleRequestId.nativeElement.textContent).toContain('req-stock');
      expect(fixture.debugElement.query(By.css('.sr-only .action-request-id'))).toBeNull();

      component.print();
      expect(component.actionError()).toBeNull();
      expect(component.requestId()).toBeNull();

      const secondRequest = httpTestingController.expectOne(
        `${environment.billingApiUrl}/invoices/123/print`,
      );
      expect(secondRequest.request.method).toBe('POST');
      secondRequest.flush({
        id: 123,
        number: 1001,
        status: 'CLOSED',
        created_at: '2026-08-21T10:00:00Z',
        closed_at: '2026-08-21T10:05:00Z',
        items: [],
      });
    });

    it('IDEMPOTENCY_CONFLICT shows safe guidance, preserves request id, and does not automatically retry', async () => {
      component.print();
      const printRequest = httpTestingController.expectOne(
        `${environment.billingApiUrl}/invoices/123/print`,
      );
      printRequest.flush(
        {
          error: {
            code: 'IDEMPOTENCY_CONFLICT',
            message: 'Idempotency conflict',
            request_id: 'req-conflict',
          },
        },
        { status: 409, statusText: 'Conflict' },
      );
      await fixture.whenStable();

      expect(component.actionError()?.code).toBe('IDEMPOTENCY_CONFLICT');
      expect(component.actionError()?.message).toContain('Atualize os dados');
      expect(component.actionError()?.requestId).toBe('req-conflict');
      expect(component.printing()).toBe(false);
      httpTestingController.expectNone(`${environment.billingApiUrl}/invoices/123`);
    });

    describe('DOM behavior', () => {
      it('shows Finalizando text and disables finalize button while printing', async () => {
        component.print();
        fixture.detectChanges();

        expect(component.printing()).toBe(true);
        const html = fixture.nativeElement.innerHTML;
        expect(html).toContain('Finalizando...');

        const finalizeButton = fixture.debugElement.query(
          By.css('button[aria-label="Finalizar e imprimir fatura"]'),
        );
        expect(finalizeButton).toBeTruthy();
        expect(finalizeButton.nativeElement.disabled).toBe(true);

        const req = httpTestingController.expectOne(
          `${environment.billingApiUrl}/invoices/123/print`,
        );
        req.flush({
          id: 123,
          number: 1001,
          status: 'CLOSED',
          created_at: '2026-08-21T10:00:00Z',
          closed_at: '2026-08-21T10:05:00Z',
          items: [],
        });
      });

      it('shows CLOSED read-only badge and no finalize action for CLOSED invoice', () => {
        component.invoice.set(closedInvoice);
        fixture.detectChanges();

        const html = fixture.nativeElement.innerHTML;
        expect(html).toContain('Fechada — somente leitura');

        const finalizeButton = fixture.debugElement.query(
          By.css('button[aria-label="Finalizar e imprimir fatura"]'),
        );
        expect(finalizeButton).toBeNull();
      });

      it('INVOICE_NOT_OPEN error reload resolves to CLOSED and hides finalize action', async () => {
        component.print();
        fixture.detectChanges();

        const printReq = httpTestingController.expectOne(
          `${environment.billingApiUrl}/invoices/123/print`,
        );
        printReq.flush(
          { error: { code: 'INVOICE_NOT_OPEN', message: 'Invoice not open' } },
          { status: 409, statusText: 'Conflict' },
        );

        const reloadReq = httpTestingController.expectOne(
          `${environment.billingApiUrl}/invoices/123`,
        );
        reloadReq.flush({
          id: 123,
          number: 1001,
          status: 'CLOSED',
          created_at: '2026-08-21T10:00:00Z',
          closed_at: '2026-08-21T10:05:00Z',
          items: [],
        });

        await fixture.whenStable();
        fixture.detectChanges();

        expect(component.invoice()?.status).toBe('CLOSED');
        const html = fixture.nativeElement.innerHTML;
        expect(html).toContain('Fechada — somente leitura');

        const finalizeButton = fixture.debugElement.query(
          By.css('button[aria-label="Finalizar e imprimir fatura"]'),
        );
        expect(finalizeButton).toBeNull();
      });

      it('print is not invoked on failures', async () => {
        windowPrintCalls = [];
        component.print();
        fixture.detectChanges();

        const req = httpTestingController.expectOne(
          `${environment.billingApiUrl}/invoices/123/print`,
        );
        req.flush(
          { error: { code: 'STOCK_SERVICE_UNAVAILABLE', message: 'Stock service unavailable' } },
          { status: 503, statusText: 'Service Unavailable' },
        );

        await fixture.whenStable();

        expect(windowPrintCalls.length).toBe(0);
      });
    });
  });
});
