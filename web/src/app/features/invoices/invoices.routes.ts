import { Routes } from '@angular/router';

export const INVOICE_ROUTES: Routes = [
  {
    path: '',
    loadComponent: () => import('./pages/invoice-list/invoice-list').then(m => m.InvoiceList)
  },
  {
    path: 'new',
    loadComponent: () => import('./pages/invoice-create/invoice-create').then(m => m.InvoiceCreate)
  },
  {
    path: ':id',
    loadComponent: () => import('./pages/invoice-detail/invoice-detail').then(m => m.InvoiceDetail)
  }
];
