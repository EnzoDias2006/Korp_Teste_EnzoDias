import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';

import { Invoice } from '../../../core/models/invoice.model';
import { environment } from '../../../../environments/environment';
import { InvoiceApiService } from './invoice-api.service';
import {
  InvoiceDetailResponseDto,
  InvoiceSummaryResponseDto,
  InvoiceItemResponseDto,
} from '../../../core/models/invoice.model';

describe('InvoiceApiService', () => {
  let service: InvoiceApiService;
  let httpTesting: HttpTestingController;

  const invoicesUrl = `${environment.billingApiUrl}/invoices`;

  const invoiceItemDto: InvoiceItemResponseDto = {
    product_id: 17,
    product_code: 'KEYBOARD-001',
    product_description: 'Mechanical keyboard',
    quantity: 2,
  };

  const invoiceDetailDto: InvoiceDetailResponseDto = {
    id: 42,
    number: 1,
    status: 'OPEN',
    items: [invoiceItemDto],
    created_at: '2026-08-20T12:00:00Z',
    closed_at: null,
  };

  const closedInvoiceDetailDto: InvoiceDetailResponseDto = {
    id: 43,
    number: 2,
    status: 'CLOSED',
    items: [invoiceItemDto],
    created_at: '2026-08-20T13:00:00Z',
    closed_at: '2026-08-20T13:30:00Z',
  };

  const invoiceSummaryDto: InvoiceSummaryResponseDto = {
    id: 42,
    number: 1,
    status: 'OPEN',
    created_at: '2026-08-20T12:00:00Z',
    closed_at: null,
  };

  const invoice: Invoice = {
    id: invoiceDetailDto.id.toString(),
    number: invoiceDetailDto.number,
    status: invoiceDetailDto.status as 'OPEN' | 'CLOSED',
    items: [
      {
        productId: invoiceItemDto.product_id.toString(),
        productCode: invoiceItemDto.product_code,
        productDescription: invoiceItemDto.product_description,
        quantity: invoiceItemDto.quantity,
      },
    ],
    createdAt: invoiceDetailDto.created_at,
    closedAt: undefined,
  };

  const closedInvoice: Invoice = {
    id: closedInvoiceDetailDto.id.toString(),
    number: closedInvoiceDetailDto.number,
    status: closedInvoiceDetailDto.status as 'OPEN' | 'CLOSED',
    items: [
      {
        productId: invoiceItemDto.product_id.toString(),
        productCode: invoiceItemDto.product_code,
        productDescription: invoiceItemDto.product_description,
        quantity: invoiceItemDto.quantity,
      },
    ],
    createdAt: closedInvoiceDetailDto.created_at,
    closedAt: closedInvoiceDetailDto.closed_at ?? undefined,
  };

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [InvoiceApiService, provideHttpClient(), provideHttpClientTesting()],
    });

    service = TestBed.inject(InvoiceApiService);
    httpTesting = TestBed.inject(HttpTestingController);
  });

  afterEach(() => {
    httpTesting.verify();
  });

  it('creates an invoice with numeric product_id and maps the response', () => {
    const request = {
      items: [{ product_id: 17, quantity: 2 }],
    };

    service.create(request).subscribe((result) => {
      expect(result).toEqual(invoice);
    });

    const httpRequest = httpTesting.expectOne(invoicesUrl);
    expect(httpRequest.request.method).toBe('POST');
    expect(httpRequest.request.body).toEqual(request);
    httpRequest.flush(invoiceDetailDto);
  });

  it('lists invoices with summary response (no items) and maps to empty items array', () => {
    const secondSummaryDto: InvoiceSummaryResponseDto = {
      id: 44,
      number: 3,
      status: 'OPEN',
      created_at: '2026-08-20T14:00:00Z',
      closed_at: null,
    };

    service.list().subscribe((result) => {
      expect(result).toEqual([
        {
          id: invoiceSummaryDto.id.toString(),
          number: invoiceSummaryDto.number,
          status: invoiceSummaryDto.status as 'OPEN' | 'CLOSED',
          createdAt: invoiceSummaryDto.created_at,
          closedAt: undefined,
        },
        {
          id: secondSummaryDto.id.toString(),
          number: secondSummaryDto.number,
          status: secondSummaryDto.status as 'OPEN' | 'CLOSED',
          createdAt: secondSummaryDto.created_at,
          closedAt: undefined,
        },
      ]);
    });

    const httpRequest = httpTesting.expectOne(invoicesUrl);
    expect(httpRequest.request.method).toBe('GET');
    expect(httpRequest.request.body).toBeNull();
    httpRequest.flush([invoiceSummaryDto, secondSummaryDto]);
  });

  it('rejects an invoice summary response with an unknown status', () => {
    const errors: unknown[] = [];

    service.list().subscribe({
      error: (error: unknown) => errors.push(error),
    });

    const httpRequest = httpTesting.expectOne(invoicesUrl);
    httpRequest.flush([{ ...invoiceSummaryDto, status: 'PENDING' }]);

    expect(errors).toHaveLength(1);
    expect(errors[0]).toEqual(
      expect.objectContaining({ message: 'The backend returned an unsupported invoice status.' }),
    );
  });

  it('gets an invoice by id and maps the detail response', () => {
    service.get(invoice.id).subscribe((result) => {
      expect(result).toEqual(invoice);
    });

    const httpRequest = httpTesting.expectOne(`${invoicesUrl}/${invoice.id}`);
    expect(httpRequest.request.method).toBe('GET');
    expect(httpRequest.request.body).toBeNull();
    httpRequest.flush(invoiceDetailDto);
  });

  it('gets a closed invoice by id and maps closed_at to closedAt', () => {
    service.get(closedInvoice.id).subscribe((result) => {
      expect(result).toEqual(closedInvoice);
    });

    const httpRequest = httpTesting.expectOne(`${invoicesUrl}/${closedInvoice.id}`);
    expect(httpRequest.request.method).toBe('GET');
    expect(httpRequest.request.body).toBeNull();
    httpRequest.flush(closedInvoiceDetailDto);
  });

  it('rejects an invoice detail response with an unknown status', () => {
    const errors: unknown[] = [];

    service.get(invoice.id).subscribe({
      error: (error: unknown) => errors.push(error),
    });

    const httpRequest = httpTesting.expectOne(`${invoicesUrl}/${invoice.id}`);
    httpRequest.flush({ ...invoiceDetailDto, status: 'PENDING' });

    expect(errors).toHaveLength(1);
    expect(errors[0]).toEqual(
      expect.objectContaining({ message: 'The backend returned an unsupported invoice status.' }),
    );
  });

  it('prints an invoice by id and maps the response', () => {
    service.print(closedInvoice.id).subscribe((result) => {
      expect(result).toEqual(closedInvoice);
    });

    const httpRequest = httpTesting.expectOne(`${invoicesUrl}/${closedInvoice.id}/print`);
    expect(httpRequest.request.method).toBe('POST');
    expect(httpRequest.request.body).toBeNull();
    httpRequest.flush(closedInvoiceDetailDto);
  });

  it('rejects a malformed print success payload without status CLOSED and non-null closed_at', () => {
    const errors: unknown[] = [];

    service.print(invoice.id).subscribe({
      error: (error: unknown) => errors.push(error),
    });

    const httpRequest = httpTesting.expectOne(`${invoicesUrl}/${invoice.id}/print`);
    httpRequest.flush({ ...invoiceDetailDto, status: 'CLOSED', closed_at: null });

    expect(errors).toHaveLength(1);
    expect(errors[0]).toBeInstanceOf(Error);
  });

  it('rejects a print response with status OPEN and non-null closed_at', () => {
    const errors: unknown[] = [];

    service.print(invoice.id).subscribe({
      error: (error: unknown) => errors.push(error),
    });

    const httpRequest = httpTesting.expectOne(`${invoicesUrl}/${invoice.id}/print`);
    httpRequest.flush({ ...invoiceDetailDto, status: 'OPEN', closed_at: '2026-08-20T13:30:00Z' });

    expect(errors).toHaveLength(1);
    expect(errors[0]).toBeInstanceOf(Error);
  });

  it('rejects a print response with an unknown status', () => {
    const errors: unknown[] = [];

    service.print(invoice.id).subscribe({
      error: (error: unknown) => errors.push(error),
    });

    const httpRequest = httpTesting.expectOne(`${invoicesUrl}/${invoice.id}/print`);
    httpRequest.flush({
      ...invoiceDetailDto,
      status: 'PENDING',
      closed_at: '2026-08-20T13:30:00Z',
    });

    expect(errors).toHaveLength(1);
    expect(errors[0]).toEqual(
      expect.objectContaining({ message: 'The backend returned an unsupported invoice status.' }),
    );
  });

  it('prints an invoice by id with encoded special characters', () => {
    const specialId = 'inv-123/test+special#chars';
    const specialInvoiceDetailDto: InvoiceDetailResponseDto = {
      id: 99,
      number: 99,
      status: 'CLOSED',
      items: [invoiceItemDto],
      created_at: '2026-08-20T15:00:00Z',
      closed_at: '2026-08-20T15:30:00Z',
    };
    const expectedInvoice: Invoice = {
      id: specialInvoiceDetailDto.id.toString(),
      number: specialInvoiceDetailDto.number,
      status: specialInvoiceDetailDto.status as 'OPEN' | 'CLOSED',
      items: [
        {
          productId: invoiceItemDto.product_id.toString(),
          productCode: invoiceItemDto.product_code,
          productDescription: invoiceItemDto.product_description,
          quantity: invoiceItemDto.quantity,
        },
      ],
      createdAt: specialInvoiceDetailDto.created_at,
      closedAt: specialInvoiceDetailDto.closed_at ?? undefined,
    };

    service.print(specialId).subscribe((result) => {
      expect(result).toEqual(expectedInvoice);
    });

    const httpRequest = httpTesting.expectOne(
      `${invoicesUrl}/${encodeURIComponent(specialId)}/print`,
    );
    expect(httpRequest.request.method).toBe('POST');
    expect(httpRequest.request.body).toBeNull();
    httpRequest.flush(specialInvoiceDetailDto);
  });

  it('prints a CLOSED invoice and maps closed_at to closedAt', () => {
    service.print(closedInvoice.id).subscribe((result) => {
      expect(result).toEqual(closedInvoice);
      expect(result.status).toBe('CLOSED');
      expect(result.closedAt).toBe(closedInvoiceDetailDto.closed_at);
    });

    const httpRequest = httpTesting.expectOne(`${invoicesUrl}/${closedInvoice.id}/print`);
    expect(httpRequest.request.method).toBe('POST');
    expect(httpRequest.request.body).toBeNull();
    httpRequest.flush(closedInvoiceDetailDto);
  });
});
