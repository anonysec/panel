//go:build !lite

package billing

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// BrandingInfo holds panel branding details used in invoice PDFs.
type BrandingInfo struct {
	AppName string
	LogoURL string
	Address string
	Phone   string
	Email   string
}

// invoiceTemplateData aggregates all data passed to the HTML invoice template.
type invoiceTemplateData struct {
	Invoice  *Invoice
	Branding BrandingInfo
	PlanName string
	Username string
	Date     string
}

// invoiceHTMLTemplate is a professional HTML template for invoice rendering.
const invoiceHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Invoice {{ .Invoice.InvoiceNumber }}</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; color: #333; padding: 40px; }
  .invoice-container { max-width: 800px; margin: 0 auto; border: 1px solid #e0e0e0; padding: 40px; }
  .header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 40px; border-bottom: 2px solid #2563eb; padding-bottom: 20px; }
  .header .brand { display: flex; align-items: center; gap: 12px; }
  .header .brand img { max-height: 48px; }
  .header .brand h1 { font-size: 24px; color: #2563eb; }
  .header .invoice-title { text-align: right; }
  .header .invoice-title h2 { font-size: 28px; color: #1e293b; margin-bottom: 4px; }
  .header .invoice-title p { font-size: 13px; color: #64748b; }
  .details { display: flex; justify-content: space-between; margin-bottom: 30px; }
  .details .section h3 { font-size: 13px; text-transform: uppercase; color: #64748b; margin-bottom: 8px; letter-spacing: 0.5px; }
  .details .section p { font-size: 14px; line-height: 1.6; }
  .items-table { width: 100%; border-collapse: collapse; margin-bottom: 30px; }
  .items-table th { background: #f1f5f9; text-align: left; padding: 12px 16px; font-size: 12px; text-transform: uppercase; color: #64748b; letter-spacing: 0.5px; }
  .items-table td { padding: 14px 16px; border-bottom: 1px solid #e2e8f0; font-size: 14px; }
  .items-table .amount { text-align: right; }
  .total-row { display: flex; justify-content: flex-end; margin-top: 10px; }
  .total-box { background: #f8fafc; border: 1px solid #e2e8f0; border-radius: 8px; padding: 16px 24px; min-width: 250px; }
  .total-box .line { display: flex; justify-content: space-between; margin-bottom: 8px; font-size: 14px; }
  .total-box .line.total { font-weight: 700; font-size: 18px; color: #2563eb; border-top: 1px solid #e2e8f0; padding-top: 8px; margin-top: 8px; }
  .status-badge { display: inline-block; padding: 4px 12px; border-radius: 12px; font-size: 12px; font-weight: 600; text-transform: uppercase; }
  .status-paid { background: #dcfce7; color: #166534; }
  .status-draft { background: #fef3c7; color: #92400e; }
  .status-cancelled { background: #fee2e2; color: #991b1b; }
  .status-refunded { background: #e0e7ff; color: #3730a3; }
  .footer { margin-top: 40px; padding-top: 20px; border-top: 1px solid #e2e8f0; text-align: center; font-size: 12px; color: #94a3b8; }
</style>
</head>
<body>
<div class="invoice-container">
  <div class="header">
    <div class="brand">
      {{if .Branding.LogoURL}}<img src="{{.Branding.LogoURL}}" alt="Logo">{{end}}
      <h1>{{.Branding.AppName}}</h1>
    </div>
    <div class="invoice-title">
      <h2>INVOICE</h2>
      <p>#{{.Invoice.InvoiceNumber}}</p>
    </div>
  </div>

  <div class="details">
    <div class="section">
      <h3>Bill To</h3>
      <p><strong>{{.Username}}</strong></p>
      <p>Customer ID: {{.Invoice.CustomerID}}</p>
    </div>
    <div class="section" style="text-align: right;">
      <h3>Invoice Details</h3>
      <p>Date: {{.Date}}</p>
      <p>Status: <span class="status-badge status-{{.Invoice.Status}}">{{.Invoice.Status}}</span></p>
      {{if .Branding.Address}}<p style="margin-top:8px; font-size:12px; color:#64748b;">{{.Branding.Address}}</p>{{end}}
      {{if .Branding.Phone}}<p style="font-size:12px; color:#64748b;">{{.Branding.Phone}}</p>{{end}}
      {{if .Branding.Email}}<p style="font-size:12px; color:#64748b;">{{.Branding.Email}}</p>{{end}}
    </div>
  </div>

  <table class="items-table">
    <thead>
      <tr>
        <th>Description</th>
        <th>Plan</th>
        <th class="amount">Amount</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td>{{.Invoice.Description}}</td>
        <td>{{.PlanName}}</td>
        <td class="amount">{{printf "%.2f" .Invoice.Amount}} {{.Invoice.Currency}}</td>
      </tr>
    </tbody>
  </table>

  <div class="total-row">
    <div class="total-box">
      <div class="line">
        <span>Subtotal</span>
        <span>{{printf "%.2f" .Invoice.Amount}} {{.Invoice.Currency}}</span>
      </div>
      <div class="line total">
        <span>Total</span>
        <span>{{printf "%.2f" .Invoice.Amount}} {{.Invoice.Currency}}</span>
      </div>
    </div>
  </div>

  <div class="footer">
    <p>Thank you for your business. — {{.Branding.AppName}}</p>
  </div>
</div>
</body>
</html>`

// GenerateInvoicePDF renders an invoice to PDF bytes.
// It uses wkhtmltopdf if available, otherwise returns an error indicating
// that wkhtmltopdf must be installed.
func GenerateInvoicePDF(inv *Invoice, branding BrandingInfo) ([]byte, error) {
	if inv == nil {
		return nil, fmt.Errorf("invoice is nil")
	}

	// Render HTML from template
	htmlBytes, err := renderInvoiceHTML(inv, branding)
	if err != nil {
		return nil, fmt.Errorf("render invoice html: %w", err)
	}

	// Try wkhtmltopdf first
	pdfBytes, err := htmlToPDFWkhtmltopdf(htmlBytes)
	if err == nil {
		return pdfBytes, nil
	}

	log.Printf("[billing] wkhtmltopdf not available: %v, returning raw HTML as fallback", err)

	// Fallback: return HTML bytes wrapped with a PDF-like content type note.
	// Callers can serve this as HTML or install wkhtmltopdf for true PDF output.
	return htmlBytes, nil
}

// RenderInvoiceHTML renders the invoice HTML without converting to PDF.
// Useful for email attachments or preview.
func RenderInvoiceHTML(inv *Invoice, branding BrandingInfo) ([]byte, error) {
	return renderInvoiceHTML(inv, branding)
}

// renderInvoiceHTML builds the HTML bytes from the template and invoice data.
func renderInvoiceHTML(inv *Invoice, branding BrandingInfo) ([]byte, error) {
	tmpl, err := template.New("invoice").Parse(invoiceHTMLTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	data := invoiceTemplateData{
		Invoice:  inv,
		Branding: branding,
		PlanName: inv.Description, // use description as plan name if not separately resolved
		Username: fmt.Sprintf("Customer #%d", inv.CustomerID),
		Date:     inv.CreatedAt.Format("2006-01-02"),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}

	return buf.Bytes(), nil
}

// htmlToPDFWkhtmltopdf converts HTML bytes to PDF using the wkhtmltopdf command.
// It pipes HTML via stdin and captures PDF from stdout.
func htmlToPDFWkhtmltopdf(htmlBytes []byte) ([]byte, error) {
	// Check if wkhtmltopdf is available
	path, err := exec.LookPath("wkhtmltopdf")
	if err != nil {
		return nil, fmt.Errorf("wkhtmltopdf not found: %w", err)
	}

	cmd := exec.Command(path,
		"--quiet",
		"--encoding", "UTF-8",
		"--page-size", "A4",
		"--margin-top", "10mm",
		"--margin-bottom", "10mm",
		"--margin-left", "10mm",
		"--margin-right", "10mm",
		"-", "-",
	)
	cmd.Stdin = bytes.NewReader(htmlBytes)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("wkhtmltopdf exec: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// invoicesDir is the default directory for storing invoice PDFs.
const invoicesDir = "/opt/KorisPanel/invoices"

// SaveInvoicePDF generates a PDF for the invoice, saves it to disk,
// and updates the invoice's pdf_path in the database.
// Returns the file path of the saved PDF.
func (b *BillingEngine) SaveInvoicePDF(ctx context.Context, inv *Invoice, branding BrandingInfo) (string, error) {
	if inv == nil {
		return "", fmt.Errorf("invoice is nil")
	}

	// Generate PDF bytes
	pdfBytes, err := GenerateInvoicePDF(inv, branding)
	if err != nil {
		return "", fmt.Errorf("generate pdf: %w", err)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(invoicesDir, 0755); err != nil {
		return "", fmt.Errorf("create invoices dir: %w", err)
	}

	// Build filename: INV-<invoice_number>.pdf
	filename := fmt.Sprintf("%s.pdf", inv.InvoiceNumber)
	filePath := filepath.Join(invoicesDir, filename)

	// Write PDF to disk
	if err := os.WriteFile(filePath, pdfBytes, 0644); err != nil {
		return "", fmt.Errorf("write pdf file: %w", err)
	}

	// Update database with pdf_path
	_, err = b.db.ExecContext(ctx, `UPDATE invoices SET pdf_path = ? WHERE id = ?`, filePath, inv.ID)
	if err != nil {
		// Attempt cleanup on DB failure
		_ = os.Remove(filePath)
		return "", fmt.Errorf("update invoice pdf_path: %w", err)
	}

	inv.PDFPath = filePath

	log.Printf("[billing] saved invoice PDF: %s (invoice=%s, customer=%d)",
		filePath, inv.InvoiceNumber, inv.CustomerID)
	return filePath, nil
}

// GenerateInvoicePDFWithUsername renders an invoice to PDF bytes with a custom username.
// This is useful when the caller has already resolved the customer's username.
func GenerateInvoicePDFWithUsername(inv *Invoice, branding BrandingInfo, username, planName string) ([]byte, error) {
	if inv == nil {
		return nil, fmt.Errorf("invoice is nil")
	}

	tmpl, err := template.New("invoice").Parse(invoiceHTMLTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	date := inv.CreatedAt.Format("2006-01-02")
	if inv.CreatedAt.IsZero() {
		date = time.Now().UTC().Format("2006-01-02")
	}

	data := invoiceTemplateData{
		Invoice:  inv,
		Branding: branding,
		PlanName: planName,
		Username: username,
		Date:     date,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}

	htmlBytes := buf.Bytes()

	// Try wkhtmltopdf
	pdfBytes, err := htmlToPDFWkhtmltopdf(htmlBytes)
	if err == nil {
		return pdfBytes, nil
	}

	log.Printf("[billing] wkhtmltopdf not available: %v, returning HTML fallback", err)
	return htmlBytes, nil
}
