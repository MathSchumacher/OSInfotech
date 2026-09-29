//go:build android

package main

// No Android nao existe modo ponte: o WhatsApp da loja fica no computador do
// balcao, que e onde o atendimento acontece. Sem este arquivo o APK carregaria
// a pilha inteira do protocolo sem nunca usar.
func runWhatsAppBridge() bool { return false }
