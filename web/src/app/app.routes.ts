import { Routes } from '@angular/router';

export const routes: Routes = [
  { path: '', redirectTo: 'products', pathMatch: 'full' },
  { path: 'products', loadChildren: () => import('./features/products/products.routes').then(m => m.PRODUCT_ROUTES) },
  { path: 'invoices', loadChildren: () => import('./features/invoices/invoices.routes').then(m => m.INVOICE_ROUTES) }
];
