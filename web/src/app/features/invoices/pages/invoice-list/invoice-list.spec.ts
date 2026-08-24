import { ComponentFixture, TestBed } from '@angular/core/testing';
import { InvoiceList } from './invoice-list';
import { InvoiceApiService } from '../../services/invoice-api.service';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { By } from '@angular/platform-browser';
import { environment } from '../../../../../environments/environment';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';
import { InvoiceSummaryResponseDto } from '../../../../core/models/invoice.model';

describe('InvoiceList', () => {
  let component: InvoiceList;
  let fixture: ComponentFixture<InvoiceList>;
  let httpTestingController: HttpTestingController;

  const invoicesUrl = `${environment.billingApiUrl}/invoices`;

  const mockInvoices: InvoiceSummaryResponseDto[] = [
    {
      id: 1,
      number: 1001,
      status: 'OPEN',
      created_at: '2026-08-21T10:00:00Z',
      closed_at: null,
    },
    {
      id: 2,
      number: 1002,
      status: 'CLOSED',
      created_at: '2026-08-21T11:00:00Z',
      closed_at: '2026-08-21T12:00:00Z',
    },
  ];

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [InvoiceList, NoopAnimationsModule],
      providers: [
        InvoiceApiService,
        provideHttpClient(),
        provideHttpClientTesting(),
        provideRouter([]),
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(InvoiceList);
    component = fixture.componentInstance;
    httpTestingController = TestBed.inject(HttpTestingController);
  });

  afterEach(() => {
    httpTestingController.verify();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });

  it('should show loading state initially', () => {
    fixture.detectChanges(); // ngOnInit triggers loadInvoices

    const loadingState = fixture.debugElement.query(By.css('.loading-state'));
    expect(loadingState).toBeTruthy();

    const req = httpTestingController.expectOne(invoicesUrl);
    req.flush([]);
  });

  it('should display empty state when no invoices exist', () => {
    fixture.detectChanges();

    const req = httpTestingController.expectOne(invoicesUrl);
    req.flush([]);

    fixture.detectChanges();

    const emptyState = fixture.debugElement.query(By.css('.empty-state'));
    expect(emptyState).toBeTruthy();
    expect(emptyState.nativeElement.textContent).toContain('No invoices found');
  });

  it('should render invoice data correctly and translate status preserving contract', () => {
    fixture.detectChanges();

    const req = httpTestingController.expectOne(invoicesUrl);
    req.flush(mockInvoices);

    fixture.detectChanges();

    const rows = fixture.debugElement.queryAll(By.css('tr[mat-row]'));
    expect(rows.length).toBe(2);

    const firstRowText = rows[0].nativeElement.textContent;
    expect(firstRowText).toContain('1001');
    expect(firstRowText).toContain('Aberta');
    expect(firstRowText).toContain('(OPEN)'); // Preserved contract in sr-only

    const secondRowText = rows[1].nativeElement.textContent;
    expect(secondRowText).toContain('1002');
    expect(secondRowText).toContain('Fechada');
    expect(secondRowText).toContain('(CLOSED)');
  });

  it('should show safe known-code error on backend validation error', () => {
    fixture.detectChanges();

    const req = httpTestingController.expectOne(invoicesUrl);
    req.flush(
      { error: { code: 'VALIDATION_ERROR', message: 'Raw backend message' } },
      { status: 400, statusText: 'Bad Request' },
    );

    fixture.detectChanges();

    const errorState = fixture.debugElement.query(By.css('.error-state'));
    expect(errorState).toBeTruthy();
    expect(errorState.nativeElement.textContent).toContain(
      'Dados inválidos. Verifique as informações e tente novamente.',
    );
    expect(errorState.nativeElement.textContent).not.toContain('Raw backend message');
  });

  it('should show fallback error on service unavailable', () => {
    fixture.detectChanges();

    const req = httpTestingController.expectOne(invoicesUrl);
    req.flush({}, { status: 0, statusText: 'Unknown Error' });

    fixture.detectChanges();

    const errorState = fixture.debugElement.query(By.css('.error-state'));
    expect(errorState).toBeTruthy();
    expect(errorState.nativeElement.textContent).toContain(
      'O serviço de estoque está indisponível. Tente novamente em instantes.',
    );
  });

  it('preserves request_id from a 503 unavailable-service response', () => {
    fixture.detectChanges();

    const req = httpTestingController.expectOne(invoicesUrl);
    req.flush(
      {
        error: {
          code: 'STOCK_SERVICE_UNAVAILABLE',
          message: 'Raw backend message',
          request_id: 'req-list-503',
        },
      },
      { status: 503, statusText: 'Service Unavailable' },
    );
    fixture.detectChanges();

    expect(component.error()).toEqual({
      code: 'STOCK_SERVICE_UNAVAILABLE',
      message: 'O serviço de estoque está indisponível. Tente novamente em instantes.',
      requestId: 'req-list-503',
    });
    expect(fixture.nativeElement.textContent).toContain('req-list-503');
  });

  it('preserves backend request_id as secondary diagnostic detail', () => {
    fixture.detectChanges();

    const req = httpTestingController.expectOne(invoicesUrl);
    req.flush(
      { error: { code: 'VALIDATION_ERROR', message: 'Raw', request_id: 'req-list' } },
      { status: 400, statusText: 'Bad Request' },
    );
    fixture.detectChanges();

    expect(component.error()?.requestId).toBe('req-list');
    expect(fixture.nativeElement.textContent).toContain('req-list');
  });
});
