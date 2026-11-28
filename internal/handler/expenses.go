package handler

import "net/http"

func (h *Handler) listExpenses(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

func (h *Handler) addExpense(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) { notBuilt(w) }

func notBuilt(w http.ResponseWriter) { http.Error(w, "not built", http.StatusNotImplemented) }
