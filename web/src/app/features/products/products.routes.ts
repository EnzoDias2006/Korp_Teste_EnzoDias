import { Routes } from '@angular/router';

export const PRODUCT_ROUTES: Routes = [
  {
    path: '',
    loadComponent: () => import('./pages/product-list/product-list').then(m => m.ProductList)
  },
  {
    path: 'new',
    loadComponent: () => import('./pages/product-create/product-create').then(m => m.ProductCreate)
  }
];
