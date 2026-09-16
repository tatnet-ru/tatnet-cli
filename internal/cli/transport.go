package cli

import (
	"crypto/x509"
	"errors"
	"fmt"
)

// explainTransport дописывает к сетевой ошибке причину, которая лежит НЕ на
// стороне TatNet.
//
// Голое «x509: certificate signed by unknown authority» читается как «у
// сервера скверный сертификат» — и уводит искать неисправность там, где её
// нет. На деле так отвечает система, в которой корневых сертификатов нет
// вовсе: замер 16.09.2026 в node:22-slim — пакет ca-certificates не
// установлен, /etc/ssl/certs пуст. npm в том же образе работает и сбивает с
// толку ещё сильнее: Node носит собственный набор корней внутри себя, а
// Go-бинарник читает системный.
//
// Корни намеренно НЕ вшиваются в клиент: вшитый набор — это снимок, который
// протухает молча и переживает отзыв удостоверяющего центра.
func explainTransport(err error) error {
	if err == nil {
		return nil
	}
	var unknown x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	if !errors.As(err, &unknown) && !errors.As(err, &invalid) {
		return err
	}
	return fmt.Errorf("%w\n\n"+
		"Проверить цепочку сертификатов нечем. Чаще всего в системе просто нет\n"+
		"корневых сертификатов — в минимальных образах (node:*-slim, alpine,\n"+
		"scratch) их ставит пакет ca-certificates:\n"+
		"  apt-get install -y ca-certificates   # debian, ubuntu\n"+
		"  apk add ca-certificates              # alpine\n"+
		"Если сертификаты на месте — значит соединение перехватывают.", err)
}
