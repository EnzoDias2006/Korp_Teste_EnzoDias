import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable, map } from 'rxjs';

import {
  CreateInvoiceRequest,
  Invoice,
  InvoiceItem,
  InvoiceDetailResponseDto,
  InvoiceStatus,
  InvoiceSummaryResponseDto,
  InvoiceSummary,
} from '../../../core/models/invoice.model';
import { environment } from '../../../../environments/environment';

@Injectable({ providedIn: 'root' })
export class InvoiceApiService {
  private readonly http = inject(HttpClient);
  private readonly invoicesUrl = `${environment.billingApiUrl}/invoices`;

  create(request: CreateInvoiceRequest): Observable<Invoice> {
    return this.http
      .post<InvoiceDetailResponseDto>(this.invoicesUrl, request)
      .pipe(map((invoice) => this.toInvoiceDetail(invoice)));
  }

  list(): Observable<InvoiceSummary[]> {
    return this.http
      .get<InvoiceSummaryResponseDto[]>(this.invoicesUrl)
      .pipe(map((invoices) => invoices.map((invoice) => this.toInvoiceSummary(invoice))));
  }

  get(id: string): Observable<Invoice> {
    return this.http
      .get<InvoiceDetailResponseDto>(`${this.invoicesUrl}/${encodeURIComponent(id)}`)
      .pipe(map((invoice) => this.toInvoiceDetail(invoice)));
  }

  print(id: string): Observable<Invoice> {
    return this.http
      .post<InvoiceDetailResponseDto>(`${this.invoicesUrl}/${encodeURIComponent(id)}/print`, null)
      .pipe(map((dto) => this.validateClosedInvoice(dto)));
  }

  private toInvoiceSummary(invoice: InvoiceSummaryResponseDto): InvoiceSummary {
    return {
      id: invoice.id.toString(),
      number: invoice.number,
      status: this.parseInvoiceStatus(invoice.status),
      createdAt: invoice.created_at,
      closedAt: invoice.closed_at ?? undefined,
    };
  }

  private toInvoiceDetail(invoice: InvoiceDetailResponseDto): Invoice {
    return {
      id: invoice.id.toString(),
      number: invoice.number,
      status: this.parseInvoiceStatus(invoice.status),
      items: invoice.items.map((item) => this.toInvoiceItem(item)),
      createdAt: invoice.created_at,
      closedAt: invoice.closed_at ?? undefined,
    };
  }

  private validateClosedInvoice(dto: InvoiceDetailResponseDto): Invoice {
    const domain = this.toInvoiceDetail(dto);
    if (
      domain.status !== 'CLOSED' ||
      typeof dto.closed_at !== 'string' ||
      dto.closed_at.length === 0
    ) {
      throw new Error('The backend did not confirm the closed invoice contract.');
    }
    return domain;
  }

  private parseInvoiceStatus(status: string): InvoiceStatus {
    if (status === 'OPEN' || status === 'CLOSED') {
      return status;
    }

    throw new Error('The backend returned an unsupported invoice status.');
  }

  private toInvoiceItem(item: {
    product_id: number;
    product_code: string;
    product_description: string;
    quantity: number;
  }): InvoiceItem {
    return {
      productId: item.product_id.toString(),
      productCode: item.product_code,
      productDescription: item.product_description,
      quantity: item.quantity,
    };
  }
}
