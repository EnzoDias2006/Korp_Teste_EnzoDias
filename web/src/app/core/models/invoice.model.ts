export type InvoiceStatus = 'OPEN' | 'CLOSED';

export interface InvoiceItem {
  productId: string;
  productCode: string;
  productDescription: string;
  quantity: number;
}

export interface Invoice {
  id: string;
  number: number;
  status: InvoiceStatus;
  items: InvoiceItem[];
  createdAt: string;
  closedAt?: string;
}

export interface InvoiceSummary {
  id: string;
  number: number;
  status: InvoiceStatus;
  createdAt: string;
  closedAt?: string;
}

export interface CreateInvoiceRequest {
  items: {
    product_id: number;
    quantity: number;
  }[];
}

// Billing API returns different response shapes
export interface InvoiceSummaryResponseDto {
  id: number;
  number: number;
  status: string;
  created_at: string;
  closed_at: string | null;
}

// Full invoice detail response (create/get) includes items
export interface InvoiceDetailResponseDto {
  id: number;
  number: number;
  status: string;
  items: InvoiceItemResponseDto[];
  created_at: string;
  closed_at: string | null;
}

// snake_case DTOs from backend
export interface InvoiceItemResponseDto {
  product_id: number;
  product_code: string;
  product_description: string;
  quantity: number;
}
