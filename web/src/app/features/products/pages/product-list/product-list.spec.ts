import { HttpErrorResponse } from '@angular/common/http';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';
import { By } from '@angular/platform-browser';
import { RouterLink, provideRouter } from '@angular/router';
import { Observable, Subject, of, throwError } from 'rxjs';
import { vi } from 'vitest';

import { ApiErrorEnvelope } from '../../../../core/models/api-error.model';
import { Product } from '../../../../core/models/product.model';
import { ProductApiService } from '../../services/product-api.service';
import { ProductList } from './product-list';

class MockProductApiService {
  readonly list = vi.fn<() => Observable<Product[]>>(() => of([]));
}

describe('ProductList', () => {
  let component: ProductList;
  let fixture: ComponentFixture<ProductList>;
  let mockProductApi: MockProductApiService;

  const products: Product[] = [
    {
      id: '1',
      code: 'PROD-001',
      description: 'Product One',
      balance: 10,
      createdAt: '2024-01-01',
      updatedAt: '2024-01-01',
    },
    {
      id: '2',
      code: 'PROD-002',
      description: 'Product Two',
      balance: 20,
      createdAt: '2024-01-02',
      updatedAt: '2024-01-02',
    },
  ];

  beforeEach(async () => {
    mockProductApi = new MockProductApiService();

    await TestBed.configureTestingModule({
      imports: [ProductList, NoopAnimationsModule],
      providers: [provideRouter([]), { provide: ProductApiService, useValue: mockProductApi }],
    }).compileComponents();
  });

  function createComponent(): void {
    fixture = TestBed.createComponent(ProductList);
    component = fixture.componentInstance;
    fixture.detectChanges();
  }

  it('creates and requests the product list once', () => {
    createComponent();

    expect(component).toBeTruthy();
    expect(mockProductApi.list).toHaveBeenCalledTimes(1);
  });

  it('shows loading feedback until the request completes', () => {
    const response = new Subject<Product[]>();
    mockProductApi.list.mockReturnValue(response);

    createComponent();

    expect(fixture.debugElement.query(By.css('mat-spinner'))).toBeTruthy();
    expect(fixture.nativeElement.textContent).toContain('Loading products...');

    response.next(products);
    response.complete();
    fixture.detectChanges();

    expect(fixture.debugElement.query(By.css('mat-spinner'))).toBeNull();
  });

  describe('loaded state with products', () => {
    beforeEach(() => {
      mockProductApi.list.mockReturnValue(of(products));
      createComponent();
    });

    it('displays the product table with the exact columns', () => {
      const table = fixture.debugElement.query(By.css('table'));
      const headerCells = fixture.debugElement.queryAll(By.css('th'));

      expect(table).toBeTruthy();
      expect(headerCells.map((cell) => cell.nativeElement.textContent.trim())).toEqual([
        'Code',
        'Description',
        'Balance',
      ]);
    });

    it('displays every product row', () => {
      const rows = fixture.debugElement.queryAll(By.css('tbody tr'));
      const firstRowCells = rows[0].queryAll(By.css('td'));

      expect(rows).toHaveLength(2);
      expect(firstRowCells.map((cell) => cell.nativeElement.textContent.trim())).toEqual([
        'PROD-001',
        'Product One',
        '10',
      ]);
    });

    it('shows an accessible link to create a product', () => {
      const createLink = fixture.debugElement.query(By.css('a[routerLink="/products/new"]'));

      expect(createLink).toBeTruthy();
      expect(createLink.nativeElement.textContent).toContain('Create Product');
      expect(fixture.debugElement.queryAll(By.directive(RouterLink))).not.toHaveLength(0);
    });
  });

  describe('empty state', () => {
    beforeEach(() => {
      mockProductApi.list.mockReturnValue(of([]));
      createComponent();
    });

    it('shows the empty card and guidance', () => {
      const emptyCard = fixture.debugElement.query(By.css('.empty-card'));

      expect(emptyCard).toBeTruthy();
      expect(emptyCard.nativeElement.textContent).toContain('No products registered yet.');
    });

    it('offers a create-product link', () => {
      const createLink = fixture.debugElement.query(
        By.css('.empty-card a[routerLink="/products/new"]'),
      );

      expect(createLink).toBeTruthy();
    });
  });

  describe('error state', () => {
    beforeEach(() => {
      mockProductApi.list.mockReturnValue(
        throwError(() =>
          createHttpError(
            'STOCK_SERVICE_UNAVAILABLE',
            'Could not load products from Stock Service.',
            503,
            'req-123',
          ),
        ),
      );
      createComponent();
    });

    it('maps a known backend code to a safe message with retry guidance', () => {
      const errorCard = fixture.debugElement.query(By.css('.error-card'));
      const retryButton = errorCard.query(By.css('button'));

      expect(errorCard).toBeTruthy();
      expect(errorCard.nativeElement.textContent).toContain(
        'O serviço de estoque está indisponível. Tente novamente em instantes.',
      );
      expect(retryButton.nativeElement.textContent).toContain('Retry');
    });

    it('does not expose machine codes, request IDs, or raw details', () => {
      const html = fixture.nativeElement.innerHTML;

      expect(html).not.toContain('STOCK_SERVICE_UNAVAILABLE');
      expect(html).toContain('ID da solicitação:');
      expect(html).toContain('req-123');
      expect(html).not.toContain('details');
    });

    it('issues one new request when retry is clicked', () => {
      mockProductApi.list.mockReturnValue(of([]));

      fixture.debugElement.query(By.css('.error-card button')).nativeElement.click();
      fixture.detectChanges();

      expect(mockProductApi.list).toHaveBeenCalledTimes(2);
      expect(fixture.debugElement.query(By.css('.error-card'))).toBeNull();
      expect(fixture.debugElement.query(By.css('.empty-card'))).toBeTruthy();
    });
  });

  it('replaces an error with authoritative products after a successful retry', () => {
    mockProductApi.list
      .mockReturnValueOnce(
        throwError(() =>
          createHttpError('STOCK_SERVICE_UNAVAILABLE', 'Could not load products.', 503),
        ),
      )
      .mockReturnValueOnce(of(products));
    createComponent();

    fixture.debugElement.query(By.css('.error-card button')).nativeElement.click();
    fixture.detectChanges();

    expect(mockProductApi.list).toHaveBeenCalledTimes(2);
    expect(fixture.debugElement.query(By.css('.error-card'))).toBeNull();
    expect(fixture.debugElement.queryAll(By.css('tbody tr'))).toHaveLength(2);
  });

  it('uses a conservative message for malformed errors', () => {
    mockProductApi.list.mockReturnValue(
      throwError(
        () =>
          new HttpErrorResponse({
            error: { internal: 'database connection string' },
            status: 500,
          }),
      ),
    );
    createComponent();

    const errorCardText = fixture.debugElement.query(By.css('.error-card')).nativeElement
      .textContent;
    expect(errorCardText).toContain('Ocorreu um erro inesperado. Tente novamente mais tarde.');
    expect(errorCardText).not.toContain('database connection string');
  });

  it('does not render internal text from a malformed nested error envelope', () => {
    mockProductApi.list.mockReturnValue(
      throwError(
        () =>
          new HttpErrorResponse({
            error: {
              error: {
                code: 'UNKNOWN_INTERNAL_ERROR',
                message: 'postgres://secret@database:5432/stock stack trace',
                details: null,
                request_id: 'req-internal',
              },
            },
            status: 500,
          }),
      ),
    );
    createComponent();

    const errorCardText = fixture.debugElement.query(By.css('.error-card')).nativeElement
      .textContent;
    expect(errorCardText).toContain('Ocorreu um erro inesperado. Tente novamente mais tarde.');
    expect(errorCardText).not.toContain('postgres://secret@database:5432/stock');
    expect(errorCardText).not.toContain('stack trace');
    expect(errorCardText).toContain('req-internal');
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
